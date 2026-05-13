package main

import (
	"flag"
	"os"
	"time"

	"go.uber.org/zap"
)

// Global application configuration derived from CLI flags
type LumoConfig struct {
	FetchDir   string
	Endpoint   string
	Service    string
	ConfigFile string
	Interval   string
	Start      time.Time
	End        time.Time
	Token      string
	Debug      bool
}

const timeFormat = "2006-01-02 15:04:05"

func parseFlags() LumoConfig {

	var cfg LumoConfig
	var startStr, endStr string

	flag.StringVar(&cfg.FetchDir, "fetch-dashboards", "", "Directory containing YAML files to fetch Grafana dashboards from")
	flag.StringVar(&cfg.Endpoint, "endpoint", "", "VictoriaMetrics endpoint URL (required)")
	flag.StringVar(&cfg.Service, "service", "", "Service name for query substitution (required)")
	flag.StringVar(&cfg.ConfigFile, "config", "graphs.json", "Path to JSON configuration file")
	flag.StringVar(&cfg.Interval, "interval", "5m", "Interval duration string for query substitution (e.g., 5m, 1h)")
	flag.StringVar(&startStr, "start", "", "Start time (YYYY-MM-DD HH:MM:SS, defaults to 24h ago)")
	flag.StringVar(&endStr, "end", "", "End time (YYYY-MM-DD HH:MM:SS, defaults to now)")
	flag.StringVar(&cfg.Token, "token", "", "Bearer token for VictoriaMetrics auth (can also use PMM_TOKEN env var)")
	flag.BoolVar(&cfg.Debug, "debug", false, "Print detailed HTTP request and response information")

	flag.Parse()
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

	return cfg
}
