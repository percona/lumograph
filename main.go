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
			zap.S().Fatalf("Node discovery failed: %v. Use the -node flag to supply the correct node name for this service.", err)
		} else {
			zap.S().Infof("Discovered node: %s", nodeName)

			cfg.Node = nodeName
		}
	}

	initFonts()

	if cfg.OutDir == "" {
		cfg.OutDir = cfg.Service
	}

	if err := os.MkdirAll(cfg.OutDir, 0o750); err != nil {
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

			// Interpolate title variables
			interpolatedTitle := strings.ReplaceAll(graphConfig.Title, "$service_name", cfg.Service)
			interpolatedTitle = strings.ReplaceAll(interpolatedTitle, "$ns_service_name", cfg.Service)

			if cfg.Node != "" {
				interpolatedTitle = strings.ReplaceAll(interpolatedTitle, "$node_name", cfg.Node)
			}

			// Override the struct title so it carries into the graph image
			graphConfig.Title = interpolatedTitle

			nameBase := graphConfig.Title

			// Deterministic Filenames: Strip any dynamic prefix (like "Service - Title")
			if parts := strings.Split(nameBase, " - "); len(parts) > 1 {
				nameBase = parts[len(parts)-1]
			}

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
		zap.S().Fatal("error: -endpoint flag is required. The base URI of the target PMM server.")
	}

	if cfg.Service == "" {
		zap.S().Fatal("error: -service flag is required. Use 'list-services' to query PMM")
	}

	if cfg.Token == "" {
		zap.S().Fatal("error: -token flag is required. (Can also set PMM_TOKEN env)")
	}

	if cfg.Groups == "" {
		zap.S().Fatal("error: -groups list is required. Use 'list-groups' to view known groups.")
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
