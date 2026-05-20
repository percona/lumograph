package main

//go:generate go run graphs/rebuild-config.go graphs

import (
	_ "embed"
	"encoding/json"
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

//go:embed graphs/graphs.json
var embeddedGraphsJSON []byte

var mediumFont = font.Font{Typeface: "Medium", Size: vg.Points(10)}
var boldFont = font.Font{Typeface: "Bold", Weight: xfont.WeightBold, Size: vg.Points(10)}

func main() {
	cmd, cfg, _ := parseFlags()

	switch cmd {
	case "list-groups":
		executeListGroups()
	case "list-services":
		executeListServices(&cfg)
	case "get-graphs":
		executeGetGraphs(&cfg)
	}
}

func executeListGroups() {

	knownGroups := GetKnownGroups(loadEmbeddedConfig())

	zap.S().Info("Available Graph Groups:")

	for g := range knownGroups {
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

	graphConfigs := loadEmbeddedConfig()

	initFonts()

	activeGroups := make(map[string]bool)
	requestedGroups := strings.Split(cfg.Groups, ",")
	knownGroups := GetKnownGroups(graphConfigs)

	for _, rg := range requestedGroups {

		rg = strings.TrimSpace(rg)
		if rg == "" {
			continue
		}

		if !knownGroups[rg] {
			zap.S().Fatalf("error: requested group '%s' does not exist in the configuration file", rg)
		}

		activeGroups[rg] = true
	}

	for _, graphConfig := range graphConfigs {

		if !activeGroups[graphConfig.Group] {
			continue
		}

		if len(graphConfig.Series) == 0 {
			zap.S().Fatalf("error: graph '%s': no series defined", graphConfig.Title)
		}

		nameBase := graphConfig.Title
		if nameBase == "" {
			nameBase = "untitled_graph"
		}

		outputFile := toSnakeCase(nameBase) + ".png"
		zap.S().Infof("Generating graph for title: %s -> %s", graphConfig.Title, outputFile)

		if err := generateGraph(cfg, &graphConfig, outputFile); err != nil {
			zap.S().Errorf("error generating graph: %v", err)
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

func loadEmbeddedConfig() []GraphConfig {

	var graphConfigs []GraphConfig
	if err := json.Unmarshal(embeddedGraphsJSON, &graphConfigs); err != nil {
		zap.S().Fatalf("error: failed to parse embedded configuration: %v", err)
	}

	if err := validateGraphConfigs(graphConfigs); err != nil {
		zap.S().Fatalf("graph config validation error: %v", err)
	}

	return graphConfigs
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
