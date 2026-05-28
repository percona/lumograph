package main

//go:generate go run rebuild-config.go resources/graphs

import (
	"maps"
	"slices"
	"sort"
	"strings"

	"go.uber.org/zap"
)

var Version = "dev"

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

	groups := slices.Collect(maps.Keys(LumoGraphs))

	sort.Strings(groups)

	for _, g := range groups {
		zap.S().Infof("  - %s", g)
	}
}

func executeListServices(cfg *LumoConfig) {

	if cfg.Endpoint == "" {
		zap.S().Fatalf("%v: -endpoint flag is required", ErrFlagRequired)
	}

	if cfg.Token == "" {
		zap.S().Fatalf("%v: -token flag is required", ErrFlagRequired)
	}

	listServices(cfg.Endpoint, cfg.Token, cfg.Debug)
}

func executeGetGraphs(cfg *LumoConfig) {

	if err := prepareGetGraphs(cfg); err != nil {
		zap.S().Fatal(err)
	}

	for graphGroup := range strings.SplitSeq(cfg.Groups, ",") {

		graphGroup = strings.TrimSpace(graphGroup)
		if graphGroup == "" {
			continue
		}

		renderGraphGroup(cfg, graphGroup)
	}
}
