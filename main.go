package main

import (
	_ "embed"
	"encoding/json"
	"os"
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

func getConfigFileData(configPath string) ([]byte, error) {
	if configPath == "" {
		// Use embedded file
		return embeddedGraphsJSON, nil
	}

	// Read from disk
	return os.ReadFile(configPath)
}

func main() {

	cmd, lumoConfig, args := parseFlags()

	switch cmd {
	case "rebuild-graphs":
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		fetchDashboards(dir)
		os.Exit(0)

	case "list-groups":
		data, err := getConfigFileData(lumoConfig.ConfigFile)
		if err != nil {
			zap.S().Fatalf("error: failed to read config data: %v", err)
		}
		var graphConfigs []GraphConfig
		if err := json.Unmarshal(data, &graphConfigs); err != nil {
			zap.S().Fatalf("error: failed to parse %s: %v", lumoConfig.ConfigFile, err)
		}

		knownGroups := GetKnownGroups(graphConfigs)

		zap.S().Info("Available Graph Groups:")
		for g := range knownGroups {
			zap.S().Infof("  - %s", g)
		}
		os.Exit(0)

	case "get-graphs":
		if lumoConfig.Endpoint == "" {
			zap.S().Fatal("error: -endpoint flag is required")
		}
		if lumoConfig.Service == "" {
			zap.S().Fatal("error: -service flag is required")
		}
		if lumoConfig.Token == "" {
			zap.S().Fatal("error: -token flag is required")
		}
		if lumoConfig.Groups == "" {
			zap.S().Fatal("error: -groups is required")
		}

		var graphConfigs []GraphConfig

		data, err := getConfigFileData(lumoConfig.ConfigFile)
		if err != nil {
			zap.S().Fatalf("error: failed to read config data: %v", err)
		}
		if err := json.Unmarshal(data, &graphConfigs); err != nil {
			zap.S().Fatalf("error: failed to parse %s: %v", lumoConfig.ConfigFile, err)
		}

		if err := validateGraphConfigs(graphConfigs); err != nil {
			zap.S().Fatalf("validation error in %s: %v", lumoConfig.ConfigFile, err)
		}

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

		activeGroups := make(map[string]bool)
		requestedGroups := strings.Split(lumoConfig.Groups, ",")

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
			if err := generateGraph(&lumoConfig, &graphConfig, outputFile); err != nil {
				zap.S().Errorf("error generating graph: %v", err)
			}
		}
	}
}
