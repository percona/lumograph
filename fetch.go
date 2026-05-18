package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// GrafanaDashboard represents the subset of Grafana JSON we need
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

func fetchDashboards(sourcePath string) {

	var files []string

	var outDir string

	if sourcePath == "" {
		sourcePath = "."
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		zap.S().Fatalf("error: source path not found: %v", err)
	}

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
		zap.S().Fatalf("error: no valid yaml source files found at %s", sourcePath)
	}

	baseUrl := "https://raw.githubusercontent.com/percona/pmm/refs/heads/v3/dashboards/dashboards/"

	var globalConfigs []GraphConfig

	for _, file := range files {

		zap.S().Infof("Processing %s...", file)

		data, err := os.ReadFile(file)
		if err != nil {
			zap.S().Infof("  Error reading file: %v", err)
			continue
		}

		var config struct {
			Dashboards []struct {
				Name   string   `yaml:"name"`
				Group  string   `yaml:"group"`
				Subdir string   `yaml:"subdir"`
				Graphs []string `yaml:"graphs"`
			} `yaml:"dashboards"`
		}

		if err := yaml.Unmarshal(data, &config); err != nil {
			zap.S().Infof("  Error parsing YAML: %v", err)
			continue
		}

		for _, dash := range config.Dashboards {

			if dash.Name == "" {
				zap.S().Error("    Dashboard missing name.")
				continue
			}

			snakeName := toSnakeCase(dash.Name)
			fileName := snakeName + ".json"

			// Construct fetch URL. If subdir exists, include it.
			subdir := dash.Subdir
			if subdir == "" {
				parts := strings.Split(dash.Name, "_")
				if len(parts) > 0 {
					subdir = parts[0]
				}
			}

			if subdir == "" {
				zap.S().Infof("    Error parsing subdir from name: '%s'", dash.Name)
				return
			}

			// Full URL to grafana dashboard
			fetchUrl := baseUrl + "/" + subdir + "/" + dash.Name + ".json"

			zap.S().Infof("  Fetching: %s -> %s", dash.Name, fileName)

			resp, err := http.Get(fetchUrl)
			if err != nil {
				zap.S().Infof("    Error fetching %s: %v", fetchUrl, err)
				continue
			}

			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				zap.S().Infof("    HTTP Error %d: Could not download %s from %s", resp.StatusCode, fileName, fetchUrl)
				continue
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				zap.S().Infof("    Error reading response: %v", err)
				continue
			}

			var grafanaDash GrafanaDashboard
			if err := json.Unmarshal(body, &grafanaDash); err != nil {
				zap.S().Infof("    Error parsing dashboard JSON: %v", err)
				continue
			}

			var lumoConfigs []GraphConfig

			// Build a fast-lookup map for the requested graphs
			wantedGraphs := make(map[string]bool)
			for _, g := range dash.Graphs {
				wantedGraphs[g] = true
			}

			for _, p := range grafanaDash.Panels {
				if p.Type != "graph" && p.Type != "timeseries" {
					continue
				}

				// Only transform graphs specifically mentioned in the YAML
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

			globalConfigs = append(globalConfigs, lumoConfigs...)

			zap.S().Infof("    Successfully downloaded and processed %s", fileName)
		}
	}

	// Save the globally aggregated configs to a single graphs.json file
	if len(globalConfigs) > 0 {
		outFileName := "graphs.json"
		outPath := filepath.Join(outDir, outFileName)

		transformed, err := json.MarshalIndent(globalConfigs, "", "  ")
		if err != nil {
			zap.S().Errorf("Error marshaling global JSON: %v", err)
			return
		}

		if err := os.WriteFile(outPath, transformed, 0644); err != nil {
			zap.S().Errorf("Error writing file %s: %v", outPath, err)
			return
		}

		zap.S().Infof("-> Successfully saved all aggregated graphs to %s", outFileName)

	} else {
		zap.S().Info("No graphs were generated.")
	}

	zap.S().Info("Done.")
}
