package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
)

// LumoConfig is the global application configuration derived from CLI flags
type LumoConfig struct {
	Endpoint string
	Service  string
	Node     string
	Groups   string
	OutDir   string
	Interval string
	Start    time.Time
	End      time.Time
	Token    string
	Debug    bool
}

const (
	timeFormat = "2006-01-02 15:04:05"

	getGraphsCommand    = "get-graphs"
	listGroupsCommand   = "list-groups"
	listServicesCommand = "list-services"
)

func parseFlags() (string, LumoConfig) {

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	command := os.Args[1]

	var cfg LumoConfig

	var startStr, endStr string

	getCmd, listCmd, listServicesCmd := setupFlagSets(&cfg, &startStr, &endStr)

	var activeCmd *flag.FlagSet

	switch command {
	case getGraphsCommand:
		activeCmd = getCmd
	case listGroupsCommand:
		activeCmd = listCmd
	case listServicesCommand:
		activeCmd = listServicesCmd
	default:
		printUsage()
		os.Exit(1)
	}

	if err := activeCmd.Parse(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing flags: %v\n", err)
		os.Exit(1)
	}

	initLogger(cfg.Debug)

	if command == getGraphsCommand || command == listServicesCommand {
		cfg.Token = resolveToken(cfg.Token)
	}

	if command == getGraphsCommand {
		cfg.Start, cfg.End = resolveTimeRanges(startStr, endStr)
	}

	return command, cfg
}

func setupFlagSets(cfg *LumoConfig, startStr, endStr *string) (*flag.FlagSet, *flag.FlagSet, *flag.FlagSet) {

	getCmd := flag.NewFlagSet(getGraphsCommand, flag.ExitOnError)
	listCmd := flag.NewFlagSet(listGroupsCommand, flag.ExitOnError)
	listServicesCmd := flag.NewFlagSet(listServicesCommand, flag.ExitOnError)

	getCmd.StringVar(&cfg.Endpoint, "endpoint", "", "PMM URL (required)")
	getCmd.StringVar(&cfg.Service, "service", "", "PMM Service name (required)")
	getCmd.StringVar(&cfg.Node, "node", "", "PMM Node name (optional)")
	getCmd.StringVar(&cfg.Groups, "groups", "", "Comma-separated list of graph groups render (required)")
	getCmd.StringVar(&cfg.OutDir, "outdir", "", "Output directory for graphs (optional, defaults to service name)")
	getCmd.StringVar(&cfg.Interval, "interval", "5m", "Interval duration for graphs (e.g., 5m, 1h)")
	getCmd.StringVar(startStr, "start", "", "Start time (YYYY-MM-DD HH:MM:SS, defaults to 24h ago)")
	getCmd.StringVar(endStr, "end", "", "End time (YYYY-MM-DD HH:MM:SS, defaults to now)")
	getCmd.StringVar(&cfg.Token, "token", "", "PMM API token (can also use PMM_TOKEN env var)")
	getCmd.BoolVar(&cfg.Debug, "debug", false, "Print detailed HTTP request and response information")

	listCmd.BoolVar(&cfg.Debug, "debug", false, "Print detailed HTTP request and response information")

	listServicesCmd.StringVar(&cfg.Endpoint, "endpoint", "", "PMM endpoint URL (required)")
	listServicesCmd.StringVar(&cfg.Token, "token", "", "Service account PMM API token (can also use PMM_TOKEN env var)")
	listServicesCmd.BoolVar(&cfg.Debug, "debug", false, "Print detailed HTTP request and response information")

	return getCmd, listCmd, listServicesCmd
}

func resolveToken(cliToken string) string {

	envToken := os.Getenv("PMM_TOKEN")
	if cliToken != "" && envToken != "" {
		zap.S().Fatalf("error: both -token flag and PMM_TOKEN environment variable are set. Please provide only one.")
	}

	if cliToken == "" {
		return envToken
	}

	return cliToken
}

func resolveTimeRanges(startStr, endStr string) (time.Time, time.Time) {

	var start, end time.Time

	var err error

	if startStr == "" {
		start = time.Now().Add(-24 * time.Hour)
	} else {
		start, err = time.ParseInLocation(timeFormat, startStr, time.Local)
		if err != nil {
			zap.S().Fatalf("error parsing -start time: %v", err)
		}
	}

	if endStr == "" {
		end = time.Now()
	} else {
		end, err = time.ParseInLocation(timeFormat, endStr, time.Local)
		if err != nil {
			zap.S().Fatalf("error parsing -end time: %v", err)
		}
	}

	return start, end
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "--- Lumograph %s ---\n", Version)
	fmt.Fprintf(os.Stderr, "Usage of %s:\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  get-graphs\t\tGenerates charts by querying a PMM endpoint.\n")
	fmt.Fprintf(os.Stderr, "  list-groups\t\tLists all available graph groups.\n")
	fmt.Fprintf(os.Stderr, "  list-services\t\tLists all available services from the PMM inventory API.\n\n")
	fmt.Fprintf(os.Stderr, "Run '%s <command> -h' to see flags for a specific command.\n", os.Args[0])
}
