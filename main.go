package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
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

func main() {

	cmd, lumoConfig, args := parseFlags()

	switch cmd {
	case "rebuild-config":
		dir := "graphs"
		if len(args) > 0 {
			dir = args[0]
		}

		fetchDashboards(dir)
		os.Exit(0)

	case "list-groups":

		var graphConfigs []GraphConfig
		if err := json.Unmarshal(embeddedGraphsJSON, &graphConfigs); err != nil {
			zap.S().Fatalf("error: failed to parse embedded configuration: %v", err)
		}

		if err := validateGraphConfigs(graphConfigs); err != nil {
			zap.S().Fatalf("graph config validation error: %v", err)
		}

		knownGroups := GetKnownGroups(graphConfigs)

		zap.S().Info("Available Graph Groups:")

		for g := range knownGroups {
			zap.S().Infof("  - %s", g)
		}

		os.Exit(0)

	case "list-services":

		if lumoConfig.Endpoint == "" {
			zap.S().Fatal("error: -endpoint flag is required")
		}

		if lumoConfig.Token == "" {
			zap.S().Fatal("error: -token flag is required")
		}

		listServices(lumoConfig.Endpoint, lumoConfig.Token, lumoConfig.Debug)
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

		// Auto-discover node name if not provided
		if lumoConfig.Node == "" {

			zap.S().Infof("Attempting to auto-discover node name for service '%s'...", lumoConfig.Service)

			nodeName, err := discoverNodeName(lumoConfig.Endpoint, lumoConfig.Token, lumoConfig.Service)
			if err != nil {
				zap.S().Warnf("Node discovery failed: %v. Continuing without $node_name substitution.", err)
			} else {
				zap.S().Infof("Discovered node: %s", nodeName)
				lumoConfig.Node = nodeName
			}
		}

		var graphConfigs []GraphConfig
		if err := json.Unmarshal(embeddedGraphsJSON, &graphConfigs); err != nil {
			zap.S().Fatalf("error: failed to parse embedded configuration: %v", err)
		}

		if err := validateGraphConfigs(graphConfigs); err != nil {
			zap.S().Fatalf("graph config validation error: %v", err)
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

type PMMService struct {
	ServiceName string `json:"service_name"`
	ServiceType string `json:"service_type"`
	NodeName    string `json:"node_name"`
}

func getPmmServices(endpoint, token string, debug bool) ([]PMMService, error) {
	req, err := http.NewRequest("GET", endpoint+"/v1/management/services", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	if debug {
		dump, err := httputil.DumpRequestOut(req, true)
		if err == nil {
			zap.S().Debugf("--- DEBUG: HTTP Request ---\n%s\n---------------------------", dump)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching services: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if debug {
		dump, err := httputil.DumpResponse(resp, true)
		if err == nil {
			zap.S().Debugf("--- DEBUG: HTTP Response ---\n%s\n---------------------------", dump)
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response struct {
		Services []PMMService `json:"services"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %w", err)
	}

	return response.Services, nil
}

// Query PMM to get a list of all services, and display the result
func listServices(endpoint, token string, debug bool) {

	services, err := getPmmServices(endpoint, token, debug)
	if err != nil {
		zap.S().Fatalf("Failed to retrieve services: %v", err)
	}

	zap.S().Info("Available Services:")

	for _, service := range services {
		if service.ServiceName != "" {
			zap.S().Infof("  - %s (%s)", service.ServiceName, serviceTypeToString(service.ServiceType))
		}
	}
}

func serviceTypeToString(sType string) string {
	if sType == "" {
		return "unknown"
	}
	return sType
}

func discoverNodeName(endpoint, token, serviceName string) (string, error) {

	services, err := getPmmServices(endpoint, token, false)
	if err != nil {
		return "", fmt.Errorf("failed to fetch services for auto-discovery: %w", err)
	}

	for _, s := range services {
		if s.ServiceName == serviceName {
			if s.NodeName == "" {
				return "", fmt.Errorf("service '%s' found, but node_name is empty", serviceName)
			}
			return s.NodeName, nil
		}
	}

	return "", fmt.Errorf("service '%s' not found", serviceName)
}
