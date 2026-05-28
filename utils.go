package main

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// formatValue takes a float64 value and returns a string formatted with commas and decimal places
func formatValue(val float64) string {

	// Get the absolute value of the input value
	absVal := math.Abs(val)

	// If the absolute value is less than 100, return the value as a string with 4 decimal places
	if absVal < 100 {
		return fmt.Sprintf("%.4f", val)
	}

	// If the absolute value is more than 100, but less than 1000,
	// return the value as a string with 2 decimal places
	if absVal < 1000 {
		return fmt.Sprintf("%.2f", val)
	}

	// If the absolute value is more than 1000, return the value as a string with no decimal places
	str := fmt.Sprintf("%.0f", val)
	if absVal <= 9999 {
		return str
	}

	// If the value is negative, set the isNegative flag to true
	isNegative := false
	if strings.HasPrefix(str, "-") {
		isNegative = true
		str = str[1:]
	}

	// Create a byte slice to store the formatted value
	var result []byte

	// Negative numbers are handled differently. Iterate backwards and construct the formatted number
	for i := len(str) - 1; i >= 0; i-- {

		if (len(str)-1-i)%3 == 0 && i != len(str)-1 {
			result = append([]byte{','}, result...)
		}

		result = append([]byte{str[i]}, result...)
	}

	if isNegative {
		result = append([]byte{'-'}, result...)
	}

	return string(result)
}

func toSnakeCase(s string) string {

	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "_")

	return strings.Trim(s, "_")
}

func interpolateGraphConfig(s string, cfg *LumoConfig) string {

	s = strings.ReplaceAll(s, "$service_name", cfg.Service)
	s = strings.ReplaceAll(s, "$ns_service_name", cfg.Service)

	s = strings.ReplaceAll(s, "$interval", cfg.Interval)

	if cfg.Node != "" {
		s = strings.ReplaceAll(s, "$node_name", cfg.Node)
	}

	if cfg.ClusterName != "" {
		s = strings.ReplaceAll(s, "$cluster", cfg.ClusterName)
		s = strings.ReplaceAll(s, "$replication_set", cfg.ClusterName)
	}

	return strings.TrimSpace(s)
}

func validateGraphConfigs(configs []GraphConfig) error {

	if len(configs) == 0 {
		return ErrEmptyConfig
	}

	for i, cfg := range configs {

		if cfg.Title == "" {
			return fmt.Errorf("graph configuration at index %d %w", i, ErrMissingTitle)
		}

		if cfg.Group == "" {
			return fmt.Errorf("graph configuration '%s' (index %d) %w", cfg.Title, i, ErrMissingGroup)
		}

		if len(cfg.Series) == 0 {
			return fmt.Errorf("graph configuration '%s' (index %d) %w", cfg.Title, i, ErrMissingSeries)
		}

		for j, series := range cfg.Series {

			if series.Legend == "" {
				return fmt.Errorf("graph configuration '%s' (index %d) series index %d %w", cfg.Title, i, j, ErrMissingLegend)
			}

			if series.Expr == "" {
				return fmt.Errorf("graph configuration '%s' (index %d) series index %d %w", cfg.Title, i, j, ErrMissingExpr)
			}
		}
	}

	return nil
}
