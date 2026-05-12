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
