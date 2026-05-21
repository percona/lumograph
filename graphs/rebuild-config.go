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

var ErrUnexpectedHTTPStatus = errors.New("unexpected HTTP status")

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
	if len(configs) == 0 {
		zap.S().Fatalf("No graph configs were processed for %s", dash.Name)
	}

	zap.S().Infof("  Successfully downloaded and processed %s", fileName)

	return configs
}

func downloadGrafanaDashboard(url, fileName string) (*GrafanaDashboard, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

	wantedGraphs := make(map[string]bool)
	for _, g := range dash.Graphs {
		wantedGraphs[g] = true
	}

	var processPanels func(panels []GrafanaPanel)

	processPanels = func(panels []GrafanaPanel) {

		for _, p := range panels {

			// Recurse into this grafana "row" which contains graphs
			if p.Type == "row" {
				processPanels(p.Panels)
				continue
			}

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

			zap.S().Infof("    Added %s graph", p.Title)
		}
	}

	processPanels(grafanaDash.Panels)

	return lumoConfigs
}

func saveGlobalConfig(configs []GraphConfig, outDir string) {

	if len(configs) == 0 {
		zap.S().Fatal("No graph configs were generated.")
	}

	grouped := make(map[string][]GraphConfig)
	for _, c := range configs {
		grouped[c.Group] = append(grouped[c.Group], c)
	}

	var sb strings.Builder
	sb.WriteString("// Code generated by rebuild-config.go. DO NOT EDIT.\n\n")
	sb.WriteString("package main\n\n")
	sb.WriteString("var LumoGraphs = map[string][]GraphConfig{\n")

	var keys []string
	for k := range grouped {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {

		fmt.Fprintf(&sb, "\t%q: {\n", k)

		for _, c := range grouped[k] {

			fmt.Fprintf(&sb, "\t\t{\n")
			fmt.Fprintf(&sb, "\t\t\tTitle: %q,\n", c.Title)
			fmt.Fprintf(&sb, "\t\t\tGroup: %q,\n", c.Group)
			fmt.Fprintf(&sb, "\t\t\tUnit:  %q,\n", c.Unit)
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

	outPath := "graphs.go"

	// #nosec
	if err := os.WriteFile(outPath, []byte(sb.String()), 0o644); err != nil {
		zap.S().Errorf("error writing %s: %v", outPath, err)
		return
	}

	zap.S().Infof("-> Successfully generated %s", outPath)
}

func toSnakeCase(s string) string {
	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}
