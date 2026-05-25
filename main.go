package main

//go:generate go run rebuild-config.go resources/graphs

import (
	_ "embed"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
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

	cmd, cfg := parseFlags()

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

	// Loop over the available graph groups
	groups := slices.Collect(maps.Keys(LumoGraphs))

	sort.Strings(groups)

	for _, g := range groups {
		zap.S().Infof("  - %s", g)
	}
}

func executeListServices(cfg *LumoConfig) {

	if cfg.Endpoint == "" {
		zap.S().Fatalf("%w: -endpoint flag is required", ErrFlagRequired)
	}

	if cfg.Token == "" {
		zap.S().Fatalf("%w: -token flag is required", ErrFlagRequired)
	}

	listServices(cfg.Endpoint, cfg.Token, cfg.Debug)
}

func executeGetGraphs(cfg *LumoConfig) {

	// Validate the flags
	err := validateGetGraphsFlags(cfg)
	if err != nil {
		zap.S().Fatalf("error: %v", err)
	}

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

	// Initialize fonts
	err = initFonts()
	if err != nil {
		zap.S().Fatalf("error initializing fonts: %v", err)
	}

	// Set output directory for graph images
	if cfg.OutDir == "" {
		cfg.OutDir = cfg.Service
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(cfg.OutDir, 0o750); err != nil {
		zap.S().Fatalf("error creating output directory '%s': %v", cfg.OutDir, err)
	}

	// Loop over each group of graphs
	for graphGroup := range strings.SplitSeq(cfg.Groups, ",") {

		graphGroup = strings.TrimSpace(graphGroup)
		if graphGroup == "" {
			continue
		}

		graphConfigs, exists := LumoGraphs[graphGroup]
		if !exists {
			zap.S().Errorf("error: requested group '%s' does not exist in the predefined configurations", graphGroup)
			continue
		}

		if err := validateGraphConfigs(graphConfigs); err != nil {
			zap.S().Errorf("validation error in group '%s': %v", graphGroup, err)
			continue
		}

		for _, graphConfig := range graphConfigs {

			if len(graphConfig.Series) == 0 {
				zap.S().Errorf("error: graph '%s': no series defined", graphConfig.Title)
				continue
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
