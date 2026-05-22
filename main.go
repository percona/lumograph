package main

//go:generate go run rebuild-config.go resources/graphs

import (
	_ "embed"
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

var Version = "dev"

var mediumFont = font.Font{Typeface: "Medium", Size: vg.Points(10)}
var boldFont = font.Font{Typeface: "Bold", Weight: xfont.WeightBold, Size: vg.Points(10)}

func main() {

	cmd, cfg, _ := parseFlags()

	zap.S().Infof("--- Lumograph %s ---", Version)

	switch cmd {
	case listGroupsCommand:
		executeListGroups()
	case listServicesCommand:
		executeListServices(&cfg)
	case getGraphsCommand:
		executeGetGraphs(&cfg)
	}
}

func executeListGroups() {

	zap.S().Info("Available Graph Groups:")

	for g := range LumoGraphs {
		zap.S().Infof("  - %s", g)
	}
}

func executeListServices(cfg *LumoConfig) {

	if cfg.Endpoint == "" {
		zap.S().Fatal("error: -endpoint flag is required")
	}

	if cfg.Token == "" {
		zap.S().Fatal("error: -token flag is required")
	}

	listServices(cfg.Endpoint, cfg.Token, cfg.Debug)
}

func executeGetGraphs(cfg *LumoConfig) {

	validateGetGraphsFlags(cfg)

	// Auto-discover node name if not provided
	if cfg.Node == "" {

		zap.S().Infof("Attempting to auto-discover node name for service '%s'...", cfg.Service)

		nodeName, err := discoverNodeName(cfg.Endpoint, cfg.Token, cfg.Service)
		if err != nil {
			zap.S().Warnf("Node discovery failed: %v. Continuing without $node_name substitution.", err)
		} else {
			zap.S().Infof("Discovered node: %s", nodeName)

			cfg.Node = nodeName
		}
	}

	initFonts()

	if cfg.OutDir == "" {
		cfg.OutDir = cfg.Service
	}

	if err := os.MkdirAll(cfg.OutDir, 0755); err != nil {
		zap.S().Fatalf("error creating output directory '%s': %v", cfg.OutDir, err)
	}

	for rg := range strings.SplitSeq(cfg.Groups, ",") {

		rg = strings.TrimSpace(rg)
		if rg == "" {
			continue
		}

		graphConfigs, exists := LumoGraphs[rg]
		if !exists {
			zap.S().Fatalf("error: requested group '%s' does not exist in the predefined configurations", rg)
		}

		if err := validateGraphConfigs(graphConfigs); err != nil {
			zap.S().Fatalf("validation error in group '%s': %v", rg, err)
		}

		for _, graphConfig := range graphConfigs {

			if len(graphConfig.Series) == 0 {
				zap.S().Fatalf("error: graph '%s': no series defined", graphConfig.Title)
			}

			nameBase := graphConfig.Title
			if nameBase == "" {
				nameBase = "untitled_graph"
			}

			fileName := toSnakeCase(nameBase) + ".png"
			outputFile := filepath.Join(cfg.OutDir, fileName)

			zap.S().Infof("Generating graph for title: %s -> %s", graphConfig.Title, outputFile)

			if err := generateGraph(cfg, &graphConfig, outputFile); err != nil {
				zap.S().Errorf("error generating graph: %v", err)
			}
		}
	}
}

func validateGetGraphsFlags(cfg *LumoConfig) {

	if cfg.Endpoint == "" {
		zap.S().Fatal("error: -endpoint flag is required")
	}

	if cfg.Service == "" {
		zap.S().Fatal("error: -service flag is required")
	}

	if cfg.Token == "" {
		zap.S().Fatal("error: -token flag is required")
	}

	if cfg.Groups == "" {
		zap.S().Fatal("error: -groups is required")
	}
}

func initFonts() {

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
}
