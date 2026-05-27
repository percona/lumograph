package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

func prepareGetGraphs(cfg *LumoConfig) error {

	if err := validateGetGraphsFlags(cfg); err != nil {
		return fmt.Errorf("error: %w", err)
	}

	if cfg.Node == "" {
		zap.S().Infof("Attempting to auto-discover node name for service '%s'...", cfg.Service)

		nodeName, err := discoverNodeName(cfg.Endpoint, cfg.Token, cfg.Service)
		if err != nil {
			return fmt.Errorf(
				"node discovery failed: %w. Use the -node flag to supply the correct node name for this service",
				err,
			)
		}

		zap.S().Infof("Discovered node: %s", nodeName)

		cfg.Node = nodeName
	}

	if err := initFonts(); err != nil {
		return fmt.Errorf("error initializing fonts: %w", err)
	}

	if cfg.OutDir == "" {
		cfg.OutDir = cfg.Service
	}

	if err := os.MkdirAll(cfg.OutDir, 0o750); err != nil {
		return fmt.Errorf("error creating output directory '%s': %w", cfg.OutDir, err)
	}

	return nil
}

func renderGraphGroup(cfg *LumoConfig, graphGroup string) {

	graphConfigs, exists := LumoGraphs[graphGroup]
	if !exists {
		zap.S().Errorf("error: requested group '%s' does not exist in the predefined configurations", graphGroup)
		return
	}

	if err := validateGraphConfigs(graphConfigs); err != nil {
		zap.S().Errorf("validation error in group '%s': %v", graphGroup, err)
		return
	}

	for _, graphConfig := range graphConfigs {
		renderGraph(cfg, graphConfig)
	}
}

func renderGraph(cfg *LumoConfig, graphConfig GraphConfig) {

	graphConfig.Title = interpolateGraphConfig(graphConfig.Title, cfg)
	outputFile := graphOutputPath(cfg.OutDir, graphConfig.Title)

	zap.S().Infof("Generating graph for title: %s -> %s", graphConfig.Title, outputFile)

	if err := generateGraph(cfg, &graphConfig, outputFile); err != nil {
		zap.S().Errorf("error generating graph: %v", err)
	}
}

func graphOutputPath(outDir, title string) string {

	nameBase := title

	if parts := strings.Split(nameBase, " - "); len(parts) > 1 {
		nameBase = parts[len(parts)-1]
	}

	if nameBase == "" {
		nameBase = "untitled_graph"
	}

	return filepath.Join(outDir, toSnakeCase(nameBase)+".png")
}

func validateGetGraphsFlags(cfg *LumoConfig) error {

	if cfg.Endpoint == "" {
		return fmt.Errorf("%w: -endpoint flag is required", ErrFlagRequired)
	}

	if cfg.Service == "" {
		return fmt.Errorf("%w: -service flag is required", ErrFlagRequired)
	}

	if cfg.Token == "" {
		return fmt.Errorf("%w: -token flag is required", ErrFlagRequired)
	}

	if cfg.Groups == "" {
		return fmt.Errorf("%w: -groups flag is required", ErrFlagRequired)
	}

	return nil
}

func initFonts() error {

	ttf, err := opentype.Parse(mediumFontTTF)
	if err != nil {
		return fmt.Errorf("error parsing embedded font: %w", err)
	}

	ttfBold, err := opentype.Parse(boldFontTTF)
	if err != nil {
		return fmt.Errorf("error parsing embedded bold font: %w", err)
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

	return nil
}
