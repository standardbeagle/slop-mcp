package cli

import (
	"strings"
	"unicode"
)

// toSnakeCase converts a string to snake_case.
func toSnakeCase(s string) string {
	var result strings.Builder
	prevLower := false

	for i, r := range s {
		if r == '-' || r == ' ' {
			result.WriteRune('_')
			prevLower = false
			continue
		}

		if unicode.IsUpper(r) {
			if i > 0 && prevLower {
				result.WriteRune('_')
			}
			result.WriteRune(unicode.ToLower(r))
			prevLower = false
		} else {
			result.WriteRune(r)
			prevLower = unicode.IsLower(r)
		}
	}

	return result.String()
}

// parseArrayValue parses a value that could be a string or array.
func parseArrayValue(v any) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		// Single value as array
		return []string{val}
	default:
		return nil
	}
}

// parseBoolValue parses a value as boolean.
func parseBoolValue(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "1" || val == "yes"
	case int:
		return val != 0
	case float64:
		return val != 0
	default:
		return false
	}
}

