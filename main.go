package main

import (
	_ "embed"
	"encoding/json"
	"os"

	"go.uber.org/zap"
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

	// Process command-line args/flags
	lumoConfig := parseFlags()

	if lumoConfig.FetchDir != "" {
		fetchDashboards(lumoConfig.FetchDir)
		os.Exit(0)
	}

	if lumoConfig.Endpoint == "" {
		zap.S().Fatal("error: -endpoint flag is required")
	}

	if lumoConfig.Service == "" {
		zap.S().Fatal("error: -service flag is required")
	}

	if lumoConfig.Token == "" {
		zap.S().Fatal("error: -token flag is required")
	}

	var graphConfigs []GraphConfig

	// Read configs from graphs config file
	data, err := os.ReadFile(lumoConfig.ConfigFile)
	if err != nil {
		zap.S().Fatalf("error: failed to read %s: %v", lumoConfig.ConfigFile, err)
	}
	if err := json.Unmarshal(data, &graphConfigs); err != nil {
		zap.S().Fatalf("error: failed to parse %s: %v", lumoConfig.ConfigFile, err)
	}

	// Load fonts
	ttf, err := opentype.Parse(mediumFontTTF)
	if err != nil {
		zap.S().Fatalf("error parsing embedded font: %v", err)
	}

	ttfBold, err := opentype.Parse(boldFontTTF)
	if err != nil {
		zap.S().Fatalf("error parsing embedded bold font: %v", err)
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

	for _, graphConfig := range graphConfigs {
		if len(graphConfig.Series) == 0 {
			zap.S().Errorf("skipping graph '%s': no series defined", graphConfig.Title)
			continue
		}

		nameBase := graphConfig.Title
		if nameBase == "" {
			nameBase = "untitled_graph"
		}
		outputFile := toSnakeCase(nameBase) + ".png"

		zap.S().Infof("Generating graph for title: %s -> %s", graphConfig.Title, outputFile)
		if err := generateGraph(&lumoConfig, &graphConfig, outputFile); err != nil {
			zap.S().Errorf("error generating graph: %v", err)
		}
	}
}
