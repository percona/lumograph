package main

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

func formatValue(val float64) string {
	absVal := math.Abs(val)

	if absVal < 100 {
		return fmt.Sprintf("%.4f", val)
	} else if absVal < 1000 {
		return fmt.Sprintf("%.2f", val)
	} else {
		str := fmt.Sprintf("%.0f", val)

		// Values greater than 9999 should show 0 digits of precision and use ',' separator
		if absVal <= 9999 {
			return str
		}

		isNegative := false
		if strings.HasPrefix(str, "-") {
			isNegative = true
			str = str[1:]
		}

		var result []byte
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
}

func toSnakeCase(s string) string {
	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

func validateGraphConfigs(configs []GraphConfig) error {
	for i, cfg := range configs {
		if cfg.Title == "" {
			return fmt.Errorf("graph configuration at index %d is missing a 'title'", i)
		}
		if cfg.Group == "" {
			return fmt.Errorf("graph configuration '%s' (index %d) is missing a 'group'", cfg.Title, i)
		}
		if len(cfg.Series) == 0 {
			return fmt.Errorf("graph configuration '%s' (index %d) has no series defined", cfg.Title, i)
		}

		for j, series := range cfg.Series {
			if series.Legend == "" {
				return fmt.Errorf("graph configuration '%s' (index %d) has an empty 'legend' in series index %d", cfg.Title, i, j)
			}
			if series.Expr == "" {
				return fmt.Errorf("graph configuration '%s' (index %d) has an empty 'expr' in series index %d", cfg.Title, i, j)
			}
		}
	}
	return nil
}

// GetKnownGroups extracts a unique set of all groups defined in a GraphConfig array.
func GetKnownGroups(configs []GraphConfig) map[string]bool {
	knownGroups := make(map[string]bool)
	for _, gc := range configs {
		if gc.Group != "" {
			knownGroups[gc.Group] = true
		}
	}
	return knownGroups
}
