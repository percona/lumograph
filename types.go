package main

import (
	"image/color"
)

// VMResponse represents a query response from Victoria Metrics API
type VMResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// TableRow represents a row in the legend-table of each graph
type TableRow struct {
	Legend string
	Color  color.Color
	Min    float64
	Max    float64
	Avg    float64
}

// SeriesConfig defines info for each series within a graph
type SeriesConfig struct {
	Legend string `json:"legend"`
	Expr   string `json:"expr"`
}

// GraphConfig defines a single graph image
type GraphConfig struct {
	Title  string         `json:"title"`
	Groups []string       `json:"groups"`
	Unit   string         `json:"unit,omitempty"`
	Series []SeriesConfig `json:"series"`
}
