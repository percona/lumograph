package main

import (
	"image/color"
)

// Response from Victoria Metrics API
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

// A row in the legend-table of each graph
type TableRow struct {
	Legend string
	Color  color.Color
	Min    float64
	Max    float64
	Avg    float64
}

// Config info for each series within a graph
type SeriesConfig struct {
	Legend string `json:"legend"`
	Expr   string `json:"expr"`
}

// Config for a single graph image
type GraphConfig struct {
	Title  string         `json:"title,omitempty"`
	Unit   string         `json:"unit,omitempty"`
	Series []SeriesConfig `json:"series"`
}
