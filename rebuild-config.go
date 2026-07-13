//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
)

// Base URL for downloading grafana dashboard definitions for PMM
const pmmBaseURL = "https://raw.githubusercontent.com/percona/pmm/refs/tags/v3.8.1/dashboards/dashboards/"

var (
	ErrUnexpectedHTTPStatus = errors.New("unexpected HTTP status")

	// Path to graph yaml
	graphsPath = "resources/graphs"
)

type SeriesConfig struct {
	Legend string `json:"legend"`
	Expr   string `json:"expr"`
}

type GraphConfig struct {
	Title  string         `json:"title"`
	Groups []string       `json:"groups"`
	Unit   string         `json:"unit,omitempty"`
	Series []SeriesConfig `json:"series"`
}

type GrafanaDashboard struct {
	Panels []GrafanaPanel `json:"panels"`
}

type GrafanaPanel struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	Yaxes []struct {
		Format string `json:"format"`
	} `json:"yaxes"`
	Targets []struct {
		Expr         string `json:"expr"`
		LegendFormat string `json:"legendFormat"`
	} `json:"targets"`
	Panels []GrafanaPanel `json:"panels,omitempty"`
}

type YamlConfig struct {
	Dashboards []YamlDashboard `yaml:"dashboards"`
}

type YamlDashboard struct {
	Name   string   `yaml:"name"`
	Groups []string `yaml:"groups"`
	Subdir string   `yaml:"subdir"`
	Graphs []string `yaml:"graphs"`
}

func main() {

	// Logger
	initLogger()

	// Get path to graphs yaml files
	files := resolveSourceFiles()

	var globalConfigs []GraphConfig

	// Process each yaml file
	for _, file := range files {
		zap.S().Infof("Processing %s...", file)
		configs := processYamlFile(file)
		globalConfigs = append(globalConfigs, configs...)
	}

	saveGraphConfigs(globalConfigs)

	zap.S().Info("Generation Done.")
}

func resolveSourceFiles() []string {

	// Check the path to the graphs yaml exists
	_, err := os.Stat(graphsPath)
	if err != nil {
		zap.S().Fatalf("path to graphs not found at %s: %v", graphsPath, err)
	}

	var files []string

	targetFiles := []string{"os.yaml", "mysql.yaml", "pgsql.yaml", "mongo.yaml", "valkey.yaml"}
	for _, f := range targetFiles {
		path := filepath.Join(graphsPath, f)
		if _, err := os.Stat(path); err == nil {
			files = append(files, path)
		}
	}

	if len(files) == 0 {
		zap.S().Fatalf("no valid yaml files found at %s", graphsPath)
	}

	return files
}

func processYamlFile(file string) []GraphConfig {

	data, err := os.ReadFile(file) // #nosec
	if err != nil {
		zap.S().Errorf("  error reading file %s: %v", file, err)
		return nil
	}

	var config YamlConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		zap.S().Errorf("  error parsing YAML: %v", err)
		return nil
	}

	var fileConfigs []GraphConfig

	for _, dash := range config.Dashboards {
		if dash.Name == "" {
			zap.S().Error("    Dashboard missing name.")
			continue
		}

		if len(dash.Groups) == 0 {
			zap.S().Errorf("    Dashboard '%s' missing groups.", dash.Name)
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

	// If the raw github path is different from the dashboard name
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

	fetchUrl := pmmBaseURL + subdir + "/" + dash.Name + ".json"

	zap.S().Infof("  Fetching: %s", dash.Name)

	grafanaDash, err := downloadGrafanaDashboard(fetchUrl)
	if err != nil {
		zap.S().Errorf("    %v", err)
		return nil
	}

	configs := mapGrafanaToLumo(dash, grafanaDash)
	if len(configs) == 0 {
		zap.S().Fatalf("No graph configs were processed for %s", dash.Name)
	}

	zap.S().Infof("  Successfully downloaded and processed %s", dash.Name)

	return configs
}

func downloadGrafanaDashboard(url string) (*GrafanaDashboard, error) {

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
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedHTTPStatus, resp.StatusCode)
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

	// A O(1) lookup map of the graphs we want in our config
	wantedGraphs := make(map[string]bool)
	for _, g := range dash.Graphs {
		wantedGraphs[g] = true
	}

	// Variable function definition
	var processPanels func(panels []GrafanaPanel)

	// Define function to process 'panels' from grafana JSON
	processPanels = func(panels []GrafanaPanel) {

		for _, p := range panels {

			// Recurse into this grafana "row" which contains graphs
			if p.Type == "row" {
				processPanels(p.Panels)
				continue
			}

			// Only 'timeseries' and 'graph' types are supported
			if p.Type != "timeseries" && p.Type != "graph" {
				continue
			}

			// If we don't want this graph, skip and go to next
			if !wantedGraphs[p.Title] {
				continue
			}

			unit := ""
			if len(p.Yaxes) > 0 {
				unit = p.Yaxes[0].Format
			}

			// Extract each series PromQL expression
			series := make([]SeriesConfig, 0, len(p.Targets))
			for _, t := range p.Targets {

				cleanExpr := strings.ReplaceAll(t.Expr, "\n", " ")
				cleanExpr = strings.ReplaceAll(cleanExpr, "\r", "")

				// Replace some PromQL variables that don't matter to us
				cleanExpr = strings.ReplaceAll(cleanExpr, "$environment", ".*")

				series = append(series, SeriesConfig{
					Legend: t.LegendFormat,
					Expr:   cleanExpr,
				})
			}

			// Add to our global config
			lumoConfigs = append(lumoConfigs, GraphConfig{
				Title:  p.Title,
				Groups: dash.Groups,
				Unit:   unit,
				Series: series,
			})

			zap.S().Infof("    Added %s graph", p.Title)
		}
	}

	processPanels(grafanaDash.Panels)

	return lumoConfigs
}

// saveGraphConfigs writes out graphs.go, containing the native struct definitions
// for all the graphs that LumoGraph can create
func saveGraphConfigs(configs []GraphConfig) {

	if len(configs) == 0 {
		zap.S().Fatal("No graph configs were generated.")
	}

	// "Groupify" the graphs. A single graph may belong to multiple groups,
	// in which case it appears under each of its group keys.
	grouped := make(map[string][]GraphConfig)
	for _, c := range configs {
		for _, g := range c.Groups {
			grouped[g] = append(grouped[g], c)
		}
	}

	var sb strings.Builder
	sb.WriteString("// Code generated by rebuild-config.go. DO NOT EDIT.\n\n")
	sb.WriteString("package main\n\n")
	sb.WriteString("var LumoGraphs = map[string][]GraphConfig{\n")

	var keys []string
	for k := range grouped {
		keys = append(keys, k)
	}

	// Sort by the group names
	sort.Strings(keys)

	// Loop over group names and write out graph definitions
	for _, k := range keys {

		fmt.Fprintf(&sb, "\t%q: {\n", k)

		for _, c := range grouped[k] {

			fmt.Fprintf(&sb, "\t\t{\n")
			fmt.Fprintf(&sb, "\t\t\tTitle:  %q,\n", c.Title)
			fmt.Fprintf(&sb, "\t\t\tGroups: %s,\n", formatStringSlice(c.Groups))
			fmt.Fprintf(&sb, "\t\t\tUnit:   %q,\n", c.Unit)
			fmt.Fprintf(&sb, "\t\t\tSeries: []SeriesConfig{\n")

			for _, s := range c.Series {
				fmt.Fprintf(&sb, "\t\t\t\t{\n")
				fmt.Fprintf(&sb, "\t\t\t\t\tLegend: %q,\n", s.Legend)
				fmt.Fprintf(&sb, "\t\t\t\t\tExpr:   %q,\n", s.Expr)
				fmt.Fprintf(&sb, "\t\t\t\t},\n")
			}

			fmt.Fprintf(&sb, "\t\t\t},\n")
			fmt.Fprintf(&sb, "\t\t},\n")
		}

		sb.WriteString("\t},\n")
	}

	sb.WriteString("}\n")

	// #nosec
	if err := os.WriteFile("lumographs.go", []byte(sb.String()), 0o644); err != nil {
		zap.S().Errorf("error writing lumographs.go: %v", err)
		return
	}

	zap.S().Infof("-> Successfully generated lumographs.go")
}

func initLogger() {

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
}

// formatStringSlice renders a []string as a Go slice literal, e.g. []string{"a", "b"}
func formatStringSlice(items []string) string {

	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}

	return "[]string{" + strings.Join(quoted, ", ") + "}"
}

func toSnakeCase(s string) string {
	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}
