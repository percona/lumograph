package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

// LumoConfig is the global application configuration derived from CLI flags
type LumoConfig struct {
	Endpoint        string
	Service         string
	Node            string
	ClusterName     string
	Database        string
	ReplSet         string
	Groups          map[string]struct{}
	OutDir          string
	Interval        string
	Start           time.Time
	End             time.Time
	Token           string
	DipperToken     string
	DipperProjectID string
	Hostname        string
	SyncDir         string
	Debug           bool
	InsecureTLS     bool
}

const (
	timeFormat = "2006-01-02 15:04:05"

	getGraphsCommand    = "get-graphs"
	listGroupsCommand   = "list-groups"
	listServicesCommand = "list-services"
	dipperSyncCommand   = "dipper-sync"
)

func parseFlags() (string, LumoConfig) {

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	command := os.Args[1]

	var cfg LumoConfig

	var startStr, endStr, groupsStr string

	getCmd, listCmd, listServicesCmd, dipperSyncCmd := setupFlagSets(&cfg, &startStr, &endStr, &groupsStr)

	var activeCmd *flag.FlagSet

	switch command {
	case getGraphsCommand:
		activeCmd = getCmd
	case listGroupsCommand:
		activeCmd = listCmd
	case listServicesCommand:
		activeCmd = listServicesCmd
	case dipperSyncCommand:
		activeCmd = dipperSyncCmd
	default:
		printUsage()
		os.Exit(1)
	}

	if err := activeCmd.Parse(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing flags: %v\n", err)
		os.Exit(1)
	}

	initLogger(cfg.Debug)

	configureHTTPClient(cfg.InsecureTLS)

	if command == getGraphsCommand || command == listServicesCommand {
		cfg.Token = resolveToken(cfg.Token, "PMM_TOKEN")
	}

	if command == getGraphsCommand {
		cfg.Start, cfg.End = resolveTimeRanges(startStr, endStr)
		cfg.Groups = parseGroups(groupsStr)
	}

	if command == dipperSyncCommand {
		cfg.DipperToken = resolveToken(cfg.DipperToken, "DIPPER_TOKEN")

		// The sole positional argument is the directory of images to upload
		cfg.SyncDir = activeCmd.Arg(0)
	}

	return command, cfg
}

func setupFlagSets(cfg *LumoConfig, startStr, endStr, groupsStr *string) (*flag.FlagSet, *flag.FlagSet, *flag.FlagSet, *flag.FlagSet) {

	getGraphsCmd := flag.NewFlagSet(getGraphsCommand, flag.ExitOnError)
	listGroupsCmd := flag.NewFlagSet(listGroupsCommand, flag.ExitOnError)
	listServicesCmd := flag.NewFlagSet(listServicesCommand, flag.ExitOnError)
	dipperSyncCmd := flag.NewFlagSet(dipperSyncCommand, flag.ExitOnError)

	getGraphsCmd.StringVar(&cfg.Endpoint, "endpoint", "", "PMM URL (required)")
	getGraphsCmd.StringVar(&cfg.Service, "service", "", "PMM Service name (required)")
	getGraphsCmd.StringVar(&cfg.Node, "node", "", "PMM Node name (optional)")
	getGraphsCmd.StringVar(&cfg.ClusterName, "cluster-name", "", "For cluster-based graphs (ie: PXC, Mongo, etc) (optional)")
	getGraphsCmd.StringVar(&cfg.Database, "database", "", "Filter for PostgreSQL databases (optional)")
	getGraphsCmd.StringVar(&cfg.ReplSet, "replset", "", "MongoDB replica set name (optional)")
	getGraphsCmd.StringVar(groupsStr, "groups", "", "Comma-separated list of graph groups render (required)")
	getGraphsCmd.StringVar(&cfg.OutDir, "outdir", "", "Output directory for graphs (optional, defaults to service name)")
	getGraphsCmd.StringVar(&cfg.Interval, "interval", "5m", "Interval duration for graphs (e.g., 5m, 1h)")
	getGraphsCmd.StringVar(startStr, "start", "", "Start time (YYYY-MM-DD HH:MM:SS, defaults to 24h ago)")
	getGraphsCmd.StringVar(endStr, "end", "", "End time (YYYY-MM-DD HH:MM:SS, defaults to now)")
	getGraphsCmd.StringVar(&cfg.Token, "token", "", "PMM API token (can also use PMM_TOKEN env var)")
	getGraphsCmd.BoolVar(&cfg.Debug, "debug", false, "Print detailed HTTP request and response information")
	getGraphsCmd.BoolVar(&cfg.InsecureTLS, "insecure-tls", false, "Disable TLS certificate verification (for self-signed certs)")

	listGroupsCmd.BoolVar(&cfg.Debug, "debug", false, "Print detailed HTTP request and response information")
	listGroupsCmd.BoolVar(&cfg.InsecureTLS, "insecure-tls", false, "Disable TLS certificate verification (for self-signed certs)")

	listServicesCmd.StringVar(&cfg.Endpoint, "endpoint", "", "PMM endpoint URL (required)")
	listServicesCmd.StringVar(&cfg.Token, "token", "", "Service account PMM API token (can also use PMM_TOKEN env var)")
	listServicesCmd.BoolVar(&cfg.Debug, "debug", false, "Print detailed HTTP request and response information")
	listServicesCmd.BoolVar(&cfg.InsecureTLS, "insecure-tls", false, "Disable TLS certificate verification (for self-signed certs)")

	dipperSyncCmd.StringVar(&cfg.DipperToken, "token", "", "Dipper API token (required, can also use DIPPER_TOKEN env var)")
	dipperSyncCmd.StringVar(&cfg.DipperProjectID, "projectid", "", "Dipper project ID (required)")
	dipperSyncCmd.StringVar(&cfg.Hostname, "hostname", "", "Hostname associated with the images (required)")

	dipperSyncCmd.Usage = dipperSyncUsage(dipperSyncCmd)

	return getGraphsCmd, listGroupsCmd, listServicesCmd, dipperSyncCmd
}

// dipperSyncUsage returns a usage function that documents the positional argument.
func dipperSyncUsage(fs *flag.FlagSet) func() {

	return func() {
		fmt.Fprintf(os.Stderr, "Usage: %s dipper-sync [flags] <image-directory>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Compresses the images in <image-directory> and uploads them to Dipper.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
	}
}

var (
	dipperTokenRe     = regexp.MustCompile(`^dipper_[a-zA-Z0-9]{41}$`)
	dipperProjectIDRe = regexp.MustCompile(`^(CS|RITM|PS)`)
)

// resolveToken returns the token from the -token flag, falling back to the
// named environment variable. Providing both is an error.
func resolveToken(cliToken, envVar string) string {

	envToken := os.Getenv(envVar)
	if cliToken != "" && envToken != "" {
		zap.S().Fatalf("error: both -token flag and %s environment variable are set. Please provide only one.", envVar)
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

// parseGroups splits the comma-separated -groups flag into a set, trimming
// whitespace and discarding empty entries.
func parseGroups(s string) map[string]struct{} {

	groups := make(map[string]struct{})

	for g := range strings.SplitSeq(s, ",") {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}

		groups[g] = struct{}{}
	}

	return groups
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "--- Lumograph %s ---\n", Version)
	fmt.Fprintf(os.Stderr, "Usage of %s:\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  get-graphs\t\tGenerates charts by querying a PMM endpoint.\n")
	fmt.Fprintf(os.Stderr, "  list-groups\t\tLists all available graph groups.\n")
	fmt.Fprintf(os.Stderr, "  list-services\t\tLists all available services from the PMM inventory API.\n")
	fmt.Fprintf(os.Stderr, "  dipper-sync\t\tCompresses a directory of images and uploads them to Dipper.\n\n")
	fmt.Fprintf(os.Stderr, "Run '%s <command> -h' to see flags for a specific command.\n", os.Args[0])
}
