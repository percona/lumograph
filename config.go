package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
)

// Global application configuration derived from CLI flags
type LumoConfig struct {
	Endpoint   string
	Service    string
	Groups     string
	ConfigFile string
	Interval   string
	Start      time.Time
	End        time.Time
	Token      string
	Debug      bool
}

const timeFormat = "2006-01-02 15:04:05"

func parseFlags() (string, LumoConfig, []string) {

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	command := os.Args[1]

	var cfg LumoConfig
	var startStr, endStr string

	// Create subcommands
	getCmd := flag.NewFlagSet("get-graphs", flag.ExitOnError)
	rebuildCmd := flag.NewFlagSet("rebuild-graphs", flag.ExitOnError)
	listCmd := flag.NewFlagSet("list-groups", flag.ExitOnError)

	// Flags for get-graphs
	getCmd.StringVar(&cfg.Endpoint, "endpoint", "", "PMM URL (required)")
	getCmd.StringVar(&cfg.Service, "service", "", "PMM Service name (required)")
	getCmd.StringVar(&cfg.Groups, "groups", "", "Comma-separated list of graph groups render (required)")
	getCmd.StringVar(&cfg.ConfigFile, "config", "graphs.json", "Path to alternate configuration file")
	getCmd.StringVar(&cfg.Interval, "interval", "5m", "Interval duration for graphs (e.g., 5m, 1h)")
	getCmd.StringVar(&startStr, "start", "", "Start time (YYYY-MM-DD HH:MM:SS, defaults to 24h ago)")
	getCmd.StringVar(&endStr, "end", "", "End time (YYYY-MM-DD HH:MM:SS, defaults to now)")
	getCmd.StringVar(&cfg.Token, "token", "", "PMM API token (can also use PMM_TOKEN env var)")
	getCmd.BoolVar(&cfg.Debug, "debug", false, "Print detailed HTTP request and response information")

	// Flags for list-groups
	listCmd.StringVar(&cfg.ConfigFile, "config", "", "Path to alternate configuration file")
	listCmd.BoolVar(&cfg.Debug, "debug", false, "Print detailed HTTP request and response information")

	// Flags for rebuild-graphs
	rebuildCmd.BoolVar(&cfg.Debug, "debug", false, "Print detailed HTTP request and response information")

	var parsedArgs []string

	switch command {
	case "get-graphs":
		getCmd.Parse(os.Args[2:])
		parsedArgs = getCmd.Args()

		initLogger(cfg.Debug)

		// Handle Token Logic
		envToken := os.Getenv("PMM_TOKEN")
		if cfg.Token != "" && envToken != "" {
			zap.S().Fatalf("error: both -token flag and PMM_TOKEN environment variable are set. Please provide only one.")
		}
		if cfg.Token == "" {
			cfg.Token = envToken
		}

		// Handle Start Time
		if startStr == "" {
			cfg.Start = time.Now().Add(-24 * time.Hour)
		} else {
			t, err := time.ParseInLocation(timeFormat, startStr, time.Local)
			if err != nil {
				zap.S().Fatalf("error parsing -start time: %v", err)
			}
			cfg.Start = t
		}

		// Handle End Time
		if endStr == "" {
			cfg.End = time.Now()
		} else {
			t, err := time.ParseInLocation(timeFormat, endStr, time.Local)
			if err != nil {
				zap.S().Fatalf("error parsing -end time: %v", err)
			}
			cfg.End = t
		}

	case "list-groups":
		listCmd.Parse(os.Args[2:])
		parsedArgs = listCmd.Args()
		initLogger(cfg.Debug)

	case "rebuild-graphs":
		rebuildCmd.Parse(os.Args[2:])
		parsedArgs = rebuildCmd.Args()
		initLogger(cfg.Debug)

	default:
		printUsage()
		os.Exit(1)
	}

	return command, cfg, parsedArgs
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  get-graphs\t\tGenerates charts by querying a PMM endpoint.\n")
	fmt.Fprintf(os.Stderr, "  rebuild-graphs [dir]\tRebuilds the graph configs from PMM source.\n")
	fmt.Fprintf(os.Stderr, "  list-groups\t\tLists all available graph groups.\n\n")
	fmt.Fprintf(os.Stderr, "Run '%s <command> -h' to see flags for a specific command.\n", os.Args[0])
}
