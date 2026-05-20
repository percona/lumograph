package main

import "errors"

// Configuration & Validation Errors
var (
	ErrMissingTitle  = errors.New("missing a 'title'")
	ErrMissingGroup  = errors.New("missing a 'group'")
	ErrMissingSeries = errors.New("has no series defined")
	ErrMissingLegend = errors.New("has an empty 'legend'")
	ErrMissingExpr   = errors.New("has an empty 'expr'")
	ErrEmtptyConfig  = errors.New("graph config is empty")
)

// API & Networking Errors
var (
	ErrCreateRequest    = errors.New("creating request")
	ErrExecRequest      = errors.New("executing request")
	ErrUnexpectedHTTP   = errors.New("unexpected status")
	ErrUnexpectedStatus = errors.New("unexpected HTTP status")
	ErrReadResponse     = errors.New("reading response")
	ErrAPIStatus        = errors.New("API status")
	ErrFetchServices    = errors.New("error fetching services")
)

// PMM Auto-Discovery Errors
var (
	ErrServiceNotFound = errors.New("service not found")
	ErrNodeNameEmpty   = errors.New("node_name is empty")
)

// Graphing & Plotting Errors
var (
	ErrNoValidPoints = errors.New("no valid points found")
	ErrCreateOutput  = errors.New("creating output file")
	ErrSavePlot      = errors.New("saving plot")
)

// Dashboard Fetching Errors
var (
	ErrSourceNotFound = errors.New("source path not found")
	ErrNoYamlFiles    = errors.New("no valid yaml source files found")
	ErrReadingFile    = errors.New("error reading file")
	ErrParsingYaml    = errors.New("error parsing YAML")
	ErrSubdirParse    = errors.New("error parsing subdir from name")
	ErrFetchingURL    = errors.New("error fetching URL")
	ErrHTTPDownload   = errors.New("could not download")
	ErrReadingResp    = errors.New("error reading response")
	ErrParsingJSON    = errors.New("error parsing JSON")
	ErrMarshalJSON    = errors.New("error marshaling JSON")
	ErrWritingFile    = errors.New("error writing file")
)
