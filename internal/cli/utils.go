package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
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
func parseArrayValue(v any) ([]string, error) {
	switch val := v.(type) {
	case []string:
		return val, nil
	case []any:
		result := make([]string, 0, len(val))
		for i, item := range val {
			switch item := item.(type) {
			case string:
				result = append(result, item)
			default:
				return nil, fmt.Errorf("array item %d has unsupported type %T", i, item)
			}
		}
		return result, nil
	case string:
		// Single value as array
		return []string{val}, nil
	default:
		return nil, fmt.Errorf("expected array or string, got %T", v)
	}
}

// parseBoolValue parses a value as boolean.
func parseBoolValue(v any) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(val)) {
		case "true", "1", "yes":
			return true, nil
		case "false", "0", "no":
			return false, nil
		default:
			return false, fmt.Errorf("invalid boolean value %q", val)
		}
	case int:
		return val != 0, nil
	case float64:
		return val != 0, nil
	default:
		return false, fmt.Errorf("expected boolean, got %T", v)
	}
}

func parseNumberValue(v any) (string, error) {
	switch val := v.(type) {
	case int:
		return strconv.Itoa(val), nil
	case int64:
		return strconv.FormatInt(val, 10), nil
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64), nil
	case json.Number:
		return val.String(), nil
	case string:
		if _, err := strconv.ParseFloat(val, 64); err != nil {
			return "", fmt.Errorf("invalid number value %q", val)
		}
		return val, nil
	default:
		return "", fmt.Errorf("expected number, got %T", v)
	}
}
