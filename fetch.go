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

func fetchDashboards(dirPath string) {

	files, _ := filepath.Glob(filepath.Join(dirPath, "*.yaml"))
	ymlFiles, _ := filepath.Glob(filepath.Join(dirPath, "*.yml"))
	files = append(files, ymlFiles...)

	if len(files) == 0 {
		zap.S().Info("No yaml files found in", dirPath)
		return
	}

	baseUrl := "https://raw.githubusercontent.com/percona/pmm/refs/heads/v3/dashboards/dashboards/"

	for _, file := range files {
		zap.S().Infof("Processing %s...", file)
		data, err := os.ReadFile(file)
		if err != nil {
			zap.S().Infof("  Error reading file: %v", err)
			continue
		}

		var config struct {
			Dashboards []struct {
				Name   string `yaml:"name"`
				Subdir string `yaml:"subdir"`
			} `yaml:"dashboards"`
		}

		if err := yaml.Unmarshal(data, &config); err != nil {
			zap.S().Infof("  Error parsing YAML: %v", err)
			continue
		}

		for _, dash := range config.Dashboards {
			if dash.Name == "" {
				continue
			}
			snakeName := toSnakeCase(dash.Name)
			fileName := snakeName + ".json"
			outPath := filepath.Join(dirPath, fileName)

			if _, err := os.Stat(outPath); err == nil {
				zap.S().Infof("    File %s already exists. Skipping.", fileName)
				continue
			}

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
			defer resp.Body.Close()

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
			for _, p := range grafanaDash.Panels {
				if p.Type != "graph" && p.Type != "timeseries" {
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
					Unit:   unit,
					Series: series,
				})
			}

			transformed, err := json.MarshalIndent(lumoConfigs, "", "  ")
			if err != nil {
				zap.S().Infof("    Error marshaling transformed JSON: %v", err)
				continue
			}

			if err := os.WriteFile(outPath, transformed, 0644); err != nil {
				zap.S().Infof("    Error writing file %s: %v", outPath, err)
				continue
			}

			zap.S().Infof("    Successfully downloaded and transformed %s", fileName)
		}
	}
	zap.S().Info("Done.")
}
