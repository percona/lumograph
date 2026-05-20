package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
)

type SeriesConfig struct {
	Legend string `json:"legend"`
	Expr   string `json:"expr"`
}

type GraphConfig struct {
	Title  string         `json:"title"`
	Group  string         `json:"group"`
	Unit   string         `json:"unit,omitempty"`
	Series []SeriesConfig `json:"series"`
}

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

func main() {

	// Logger
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.DisableCaller = true
	config.DisableStacktrace = true

	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)

	logger, err := config.Build()
	if err != nil {
		os.Exit(1)
	}

	zap.ReplaceGlobals(logger)

	sourcePath := "graphs"
	if len(os.Args) > 1 {
		sourcePath = os.Args[1]
	}

	files, outDir := resolveSourceFiles(sourcePath)
	var globalConfigs []GraphConfig

	for _, file := range files {
		zap.S().Infof("Processing %s...", file)
		configs := processYamlFile(file)
		globalConfigs = append(globalConfigs, configs...)
	}

	saveGlobalConfig(globalConfigs, outDir)

	zap.S().Info("Generation Done.")
}

func resolveSourceFiles(sourcePath string) ([]string, string) {

	if sourcePath == "" {
		sourcePath = "."
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		zap.S().Fatalf("source path not found at %s: %v", sourcePath, err)
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
		zap.S().Fatalf("no valid yaml source files found at %s", sourcePath)
	}

	return files, outDir
}

func processYamlFile(file string) []GraphConfig {

	var fileConfigs []GraphConfig

	data, err := os.ReadFile(file)
	if err != nil {
		zap.S().Errorf("  error reading file %s: %v", file, err)
		return nil
	}

	var config YamlConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		zap.S().Errorf("  error parsing YAML: %v", err)
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
		zap.S().Errorf("    error parsing subdir from name: '%s'", dash.Name)
		return nil
	}

	fileName := toSnakeCase(dash.Name) + ".json"
	fetchUrl := pmmBaseURL + subdir + "/" + dash.Name + ".json"

	zap.S().Infof("  Fetching: %s -> %s", dash.Name, fileName)

	grafanaDash, err := downloadGrafanaDashboard(fetchUrl, fileName)
	if err != nil {
		zap.S().Errorf("    %v", err)
		return nil
	}

	configs := mapGrafanaToLumo(dash, grafanaDash)
	zap.S().Infof("    Successfully downloaded and processed %s", fileName)

	return configs
}

func downloadGrafanaDashboard(url, fileName string) (*GrafanaDashboard, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP Error %d: could not download %s from %s", resp.StatusCode, fileName, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	var grafanaDash GrafanaDashboard
	if err := json.Unmarshal(body, &grafanaDash); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %w", err)
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
		zap.S().Fatal("No graph configs were generated.")
	}

	outFileName := "graphs.json"
	outPath := filepath.Join(outDir, outFileName)

	transformed, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		zap.S().Errorf("error marshaling JSON: %v", err)
		return
	}

	if err := os.WriteFile(outPath, transformed, 0o600); err != nil {
		zap.S().Errorf("error writing file %s: %v", outPath, err)
		return
	}

	zap.S().Infof("-> Successfully saved graph configs to %s", outFileName)
}

func toSnakeCase(s string) string {
	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}
