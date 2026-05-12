package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/opentype"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/font"
	"gonum.org/v1/plot/vg"
)

//go:embed resources/fonts/Inter_18pt-Medium.ttf
var mediumFontTTF []byte

//go:embed resources/fonts/Inter_18pt-Bold.ttf
var boldFontTTF []byte

var mediumFont = font.Font{Typeface: "Medium", Size: vg.Points(10)}
var boldFont = font.Font{Typeface: "Bold", Weight: xfont.WeightBold, Size: vg.Points(10)}

func main() {

	// Flags
	fetchDir := flag.String("fetch-dashboards", "", "Directory containing YAML files to fetch Grafana dashboards from")
	endpoint := flag.String("endpoint", "", "VictoriaMetrics endpoint URL (required)")
	service := flag.String("service", "", "Service name for query substitution (required)")
	configFile := flag.String("config", "graphs.json", "Path to JSON configuration file")
	interval := flag.String("interval", "5m", "Interval duration string for query substitution (e.g., 5m, 1h)")
	start := flag.Duration("start", time.Hour, "Start time (duration)")
	end := flag.Duration("end", 0*time.Second, "End time (duration from now, 0 = now)")
	token := flag.String("token", "", "Bearer token for VictoriaMetrics auth (required)")
	debug := flag.Bool("debug", false, "Print detailed HTTP request and response information")

	flag.Parse()

	if *fetchDir != "" {
		fetchDashboards(*fetchDir)
		os.Exit(0)
	}

	if *endpoint == "" {
		fmt.Fprintln(os.Stderr, "error: -endpoint flag is required")
		os.Exit(1)
	}

	if *service == "" {
		fmt.Fprintln(os.Stderr, "error: -service flag is required")
		os.Exit(1)
	}

	if *token == "" {
		fmt.Fprintln(os.Stderr, "error: -token flag is required")
		os.Exit(1)
	}

	endTime := time.Now().Add(-*end)
	startTime := endTime.Add(-*start)

	var configs []GraphConfig

	// Read configs from graphs config file
	data, err := os.ReadFile(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to read %s: %v\n", *configFile, err)
		os.Exit(1)
	}
	if err := json.Unmarshal(data, &configs); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to parse %s: %v\n", *configFile, err)
		os.Exit(1)
	}

	// Load fonts
	ttf, err := opentype.Parse(mediumFontTTF)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing embedded font: %v\n", err)
		os.Exit(1)
	}

	ttfBold, err := opentype.Parse(boldFontTTF)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing embedded bold font: %v\n", err)
		os.Exit(1)
	}

	font.DefaultCache.Add([]font.Face{
		{
			Font: mediumFont,
			Face: ttf,
		},
		{
			Font: boldFont,
			Face: ttfBold,
		},
	})

	plot.DefaultFont = mediumFont

	for _, cfg := range configs {
		if len(cfg.Series) == 0 {
			fmt.Fprintf(os.Stderr, "skipping graph '%s': no series defined\n", cfg.Title)
			continue
		}

		nameBase := cfg.Title
		if nameBase == "" {
			nameBase = "untitled_graph"
		}
		outputFile := toSnakeCase(nameBase) + ".png"

		fmt.Printf("Generating graph for title: %s -> %s\n", cfg.Title, outputFile)
		if err := generateGraph(*endpoint, *service, *interval, &cfg, *token, outputFile, startTime, endTime, *debug); err != nil {
			fmt.Fprintf(os.Stderr, "error generating graph: %v\n", err)
		}
	}
}
