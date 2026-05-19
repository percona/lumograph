package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

var (
	ErrSourceNotFound = errors.New("source path not found")
	ErrNoYamlFiles    = errors.New("no valid yaml source files found")
	ErrReadingFile    = errors.New("error reading file")
	ErrParsingYaml    = errors.New("error parsing YAML")
	ErrSubdirParse    = errors.New("error parsing subdir from name")
	ErrFetchingURL    = errors.New("error fetching URL")
	ErrHTTPDownload   = errors.New("could not download")
	ErrReadingResp    = errors.New("error reading response")
	ErrParsingJSON    = errors.New("error parsing dashboard JSON")
	ErrMarshalJSON    = errors.New("error marshaling global JSON")
	ErrWritingFile    = errors.New("error writing file")
)

const pmmBaseURL = "https://raw.githubusercontent.com/percona/pmm/refs/heads/v3/dashboards/dashboards/"

type GrafanaDashboard struct {
	Panels []struct {
		Type  string `json:"type"`
		Title string `json:"title"`
		Yaxes []struct {
			Format string `json:"format"`
		} `json:"yaxes"`
		Targets []struct {
			Expr         string `json:"expr"`
			LegendFormat string `json:"legendFormat"`
		} `json:"targets"`
	} `json:"panels"`
}

type YamlConfig struct {
	Dashboards []YamlDashboard `yaml:"dashboards"`
}

type YamlDashboard struct {
	Name   string   `yaml:"name"`
	Group  string   `yaml:"group"`
	Subdir string   `yaml:"subdir"`
	Graphs []string `yaml:"graphs"`
}

func fetchDashboards(sourcePath string) {

	files, outDir := resolveSourceFiles(sourcePath)

	var globalConfigs []GraphConfig

	for _, file := range files {
		zap.S().Infof("Processing %s...", file)
		configs := processYamlFile(file)
		globalConfigs = append(globalConfigs, configs...)
	}

	saveGlobalConfig(globalConfigs, outDir)

	zap.S().Info("Done.")
}

func resolveSourceFiles(sourcePath string) ([]string, string) {

	if sourcePath == "" {
		sourcePath = "."
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		zap.S().Fatalf("%v at %s: %v", ErrSourceNotFound, sourcePath, err)
	}

	var files []string

	var outDir string

	if info.IsDir() {

		outDir = sourcePath

		targetFiles := []string{"os.yaml", "mysql.yaml", "pgsql.yaml", "mongo.yaml", "valkey.yaml"}
		for _, f := range targetFiles {
			path := filepath.Join(sourcePath, f)
			if _, err := os.Stat(path); err == nil {
				files = append(files, path)
			}
		}
	} else {
		outDir = filepath.Dir(sourcePath)
		files = []string{sourcePath}
	}

	if len(files) == 0 {
		zap.S().Fatalf("%v at %s", ErrNoYamlFiles, sourcePath)
	}

	return files, outDir
}

func processYamlFile(file string) []GraphConfig {

	var fileConfigs []GraphConfig

	data, err := os.ReadFile(file)
	if err != nil {
		zap.S().Errorf("  %v %s: %v", ErrReadingFile, file, err)
		return nil
	}

	var config YamlConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		zap.S().Errorf("  %v: %v", ErrParsingYaml, err)
		return nil
	}

	for _, dash := range config.Dashboards {

		if dash.Name == "" {
			zap.S().Error("    Dashboard missing name.")
			continue
		}

		dashConfigs := fetchAndTransformDashboard(dash)
		if dashConfigs != nil {
			fileConfigs = append(fileConfigs, dashConfigs...)
		}
	}

	return fileConfigs
}

func fetchAndTransformDashboard(dash YamlDashboard) []GraphConfig {

	subdir := dash.Subdir
	if subdir == "" {
		parts := strings.Split(dash.Name, "_")
		if len(parts) > 0 {
			subdir = parts[0]
		}
	}

	if subdir == "" {
		zap.S().Errorf("    %v: '%s'", ErrSubdirParse, dash.Name)
		return nil
	}

	fileName := toSnakeCase(dash.Name) + ".json"
	fetchUrl := pmmBaseURL + subdir + "/" + dash.Name + ".json"

	zap.S().Infof("  Fetching: %s -> %s", dash.Name, fileName)

	grafanaDash, err := downloadGrafanaDashboard(fetchUrl, fileName)
	if err != nil {
		zap.S().Infof("    %v", err)
		return nil
	}

	configs := mapGrafanaToLumo(dash, grafanaDash)

	zap.S().Infof("    Successfully downloaded and processed %s", fileName)

	return configs
}

func downloadGrafanaDashboard(url, fileName string) (*GrafanaDashboard, error) {

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrFetchingURL, url, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP Error %d: %w %s from %s", resp.StatusCode, ErrHTTPDownload, fileName, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReadingResp, err)
	}

	var grafanaDash GrafanaDashboard

	if err := json.Unmarshal(body, &grafanaDash); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParsingJSON, err)
	}

	return &grafanaDash, nil
}

func mapGrafanaToLumo(dash YamlDashboard, grafanaDash *GrafanaDashboard) []GraphConfig {

	var lumoConfigs []GraphConfig

	wantedGraphs := make(map[string]bool)
	for _, g := range dash.Graphs {
		wantedGraphs[g] = true
	}

	for _, p := range grafanaDash.Panels {
		if p.Type != "graph" && p.Type != "timeseries" {
			continue
		}

		if !wantedGraphs[p.Title] {
			continue
		}

		unit := ""
		if len(p.Yaxes) > 0 {
			unit = p.Yaxes[0].Format
		}

		series := make([]SeriesConfig, 0, len(p.Targets))
		for _, t := range p.Targets {
			series = append(series, SeriesConfig{
				Legend: t.LegendFormat,
				Expr:   t.Expr,
			})
		}

		lumoConfigs = append(lumoConfigs, GraphConfig{
			Title:  p.Title,
			Group:  dash.Group,
			Unit:   unit,
			Series: series,
		})
	}

	return lumoConfigs
}

func saveGlobalConfig(configs []GraphConfig, outDir string) {

	if len(configs) == 0 {
		zap.S().Info("No graph configs were generated.")
		return
	}

	outFileName := "graphs.json"
	outPath := filepath.Join(outDir, outFileName)

	transformed, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		zap.S().Errorf("%v: %v", ErrMarshalJSON, err)
		return
	}

	if err := os.WriteFile(outPath, transformed, 0644); err != nil {
		zap.S().Errorf("%v %s: %v", ErrWritingFile, outPath, err)
		return
	}

	zap.S().Infof("-> Successfully saved graph configs to %s", outFileName)
}
