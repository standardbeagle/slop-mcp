package builtins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/standardbeagle/slop/pkg/slop"
)

// templateRuntime holds reference to SLOP runtime for callbacks
var templateRuntime *slop.Runtime

// RegisterTemplate registers template functions with the SLOP runtime.
func RegisterTemplate(rt *slop.Runtime) {
	templateRuntime = rt

	rt.RegisterBuiltin("template_render", builtinTemplateRender)
	rt.RegisterBuiltin("template_render_file", builtinTemplateRenderFile)

	// Text manipulation helpers
	rt.RegisterBuiltin("indent", builtinIndent)
	rt.RegisterBuiltin("dedent", builtinDedent)
	rt.RegisterBuiltin("wrap", builtinWrap)
}

// builtinTemplateRender renders a Go template with provided data.
// template_render(template_string, data) -> string
func builtinTemplateRender(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("template_render requires template and data arguments")
	}

	tmplStr, ok := args[0].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("template_render: template must be a string")
	}

	data := slopValueToAny(args[1])

	result, err := renderTemplate(tmplStr.Value, data)
	if err != nil {
		return nil, fmt.Errorf("template_render: %w", err)
	}

	return slop.NewStringValue(result), nil
}

// builtinTemplateRenderFile renders a Go template from a file.
// template_render_file(path, data) -> string
func builtinTemplateRenderFile(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("template_render_file requires path and data arguments")
	}

	pathStr, ok := args[0].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("template_render_file: path must be a string")
	}

	tmplContent, err := os.ReadFile(pathStr.Value)
	if err != nil {
		return nil, fmt.Errorf("template_render_file: failed to read file: %w", err)
	}

	data := slopValueToAny(args[1])

	result, err := renderTemplate(string(tmplContent), data)
	if err != nil {
		return nil, fmt.Errorf("template_render_file: %w", err)
	}

	return slop.NewStringValue(result), nil
}

// renderTemplate renders a Go template with custom functions.
func renderTemplate(tmplStr string, data any) (string, error) {
	funcMap := createFuncMap()

	tmpl, err := template.New("template").Funcs(funcMap).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute error: %w", err)
	}

	return buf.String(), nil
}

// createFuncMap creates the template function map with SLOP integration.
func createFuncMap() template.FuncMap {
	return template.FuncMap{
		// String functions
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"title":      strings.Title,
		"trim":       strings.TrimSpace,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,
		"replace":    strings.ReplaceAll,
		"split":      strings.Split,
		"join":       strings.Join,
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"repeat":     strings.Repeat,

		// Formatting
		"indent":  tmplIndent,
		"nindent": tmplNindent,
		"quote":   tmplQuote,

		// Type conversion
		"toString": tmplToString,
		"toInt":    tmplToInt,
		"toFloat":  tmplToFloat,
		"toBool":   tmplToBool,

		// List operations
		"first": tmplFirst,
		"last":  tmplLast,
		"rest":  tmplRest,
		"len":   tmplLen,

		// Map operations
		"keys":   tmplKeys,
		"values": tmplValues,
		"hasKey": tmplHasKey,
		"get":    tmplGet,

		// Conditionals
		"default": tmplDefault,
		"empty":   tmplEmpty,
		"coalesce": tmplCoalesce,

		// Math
		"add": tmplAdd,
		"sub": tmplSub,
		"mul": tmplMul,
		"div": tmplDiv,
		"mod": tmplMod,

		// SLOP callback - call any SLOP expression
		"slop": tmplSlopCall,

		// JSON
		"toJson":       tmplToJSON,
		"toPrettyJson": tmplToPrettyJSON,
		"fromJson":     tmplFromJSON,

		// YAML-like
		"toYaml": tmplToYAML,
	}
}

// Template function implementations

func tmplIndent(spaces int, v string) string {
	pad := strings.Repeat(" ", spaces)
	return pad + strings.ReplaceAll(v, "\n", "\n"+pad)
}

func tmplNindent(spaces int, v string) string {
	return "\n" + tmplIndent(spaces, v)
}

func tmplQuote(v any) string {
	return fmt.Sprintf("%q", v)
}

func tmplToString(v any) string {
	return fmt.Sprintf("%v", v)
}

func tmplToInt(v any) int64 {
	switch val := v.(type) {
	case int:
		return int64(val)
	case int64:
		return val
	case float64:
		return int64(val)
	case string:
		var i int64
		fmt.Sscanf(val, "%d", &i)
		return i
	default:
		return 0
	}
}

func tmplToFloat(v any) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case float64:
		return val
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}

func tmplToBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case int, int64, float64:
		return val != 0
	case string:
		return val != "" && val != "false" && val != "0"
	default:
		return v != nil
	}
}

func tmplFirst(list any) any {
	switch v := list.(type) {
	case []any:
		if len(v) > 0 {
			return v[0]
		}
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	}
	return nil
}

func tmplLast(list any) any {
	switch v := list.(type) {
	case []any:
		if len(v) > 0 {
			return v[len(v)-1]
		}
	case []string:
		if len(v) > 0 {
			return v[len(v)-1]
		}
	}
	return nil
}

func tmplRest(list any) any {
	switch v := list.(type) {
	case []any:
		if len(v) > 1 {
			return v[1:]
		}
		return []any{}
	case []string:
		if len(v) > 1 {
			return v[1:]
		}
		return []string{}
	}
	return nil
}

func tmplLen(v any) int {
	switch val := v.(type) {
	case string:
		return len(val)
	case []any:
		return len(val)
	case []string:
		return len(val)
	case map[string]any:
		return len(val)
	default:
		return 0
	}
}

func tmplKeys(m any) []string {
	if mapVal, ok := m.(map[string]any); ok {
		keys := make([]string, 0, len(mapVal))
		for k := range mapVal {
			keys = append(keys, k)
		}
		return keys
	}
	return nil
}

func tmplValues(m any) []any {
	if mapVal, ok := m.(map[string]any); ok {
		values := make([]any, 0, len(mapVal))
		for _, v := range mapVal {
			values = append(values, v)
		}
		return values
	}
	return nil
}

func tmplHasKey(m any, key string) bool {
	if mapVal, ok := m.(map[string]any); ok {
		_, exists := mapVal[key]
		return exists
	}
	return false
}

func tmplGet(m any, key string, defaultVal ...any) any {
	if mapVal, ok := m.(map[string]any); ok {
		if val, exists := mapVal[key]; exists {
			return val
		}
	}
	if len(defaultVal) > 0 {
		return defaultVal[0]
	}
	return nil
}

func tmplDefault(defaultVal, val any) any {
	if tmplEmpty(val) {
		return defaultVal
	}
	return val
}

func tmplEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case []any:
		return len(val) == 0
	case map[string]any:
		return len(val) == 0
	case bool:
		return !val
	case int, int64, float64:
		return val == 0
	default:
		return false
	}
}

func tmplCoalesce(vals ...any) any {
	for _, v := range vals {
		if !tmplEmpty(v) {
			return v
		}
	}
	return nil
}

func tmplAdd(a, b any) any {
	af := tmplToFloat(a)
	bf := tmplToFloat(b)
	result := af + bf
	if result == float64(int64(result)) {
		return int64(result)
	}
	return result
}

func tmplSub(a, b any) any {
	af := tmplToFloat(a)
	bf := tmplToFloat(b)
	result := af - bf
	if result == float64(int64(result)) {
		return int64(result)
	}
	return result
}

func tmplMul(a, b any) any {
	af := tmplToFloat(a)
	bf := tmplToFloat(b)
	result := af * bf
	if result == float64(int64(result)) {
		return int64(result)
	}
	return result
}

func tmplDiv(a, b any) any {
	af := tmplToFloat(a)
	bf := tmplToFloat(b)
	if bf == 0 {
		return 0
	}
	return af / bf
}

func tmplMod(a, b any) int64 {
	ai := tmplToInt(a)
	bi := tmplToInt(b)
	if bi == 0 {
		return 0
	}
	return ai % bi
}

// tmplSlopCall calls a SLOP expression and returns the result.
// Usage in template: {{ slop "upper(name)" }}
func tmplSlopCall(expr string) (any, error) {
	if templateRuntime == nil {
		return nil, fmt.Errorf("SLOP runtime not available")
	}

	result, err := templateRuntime.Execute(expr)
	if err != nil {
		return nil, err
	}

	return slopValueToAny(result), nil
}

func tmplToJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

func tmplToPrettyJSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(data)
}

func tmplFromJSON(s string) any {
	var result any
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

func tmplToYAML(v any) string {
	return toYAMLString(v, 0)
}

func toYAMLString(v any, indent int) string {
	pad := strings.Repeat("  ", indent)

	switch val := v.(type) {
	case nil:
		return "null"
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int, int64, float64:
		return fmt.Sprintf("%v", val)
	case string:
		// Check if needs quoting
		if strings.ContainsAny(val, ":#{}[]&*!|>'\"%@`") || val == "" {
			return fmt.Sprintf("%q", val)
		}
		return val
	case []any:
		if len(val) == 0 {
			return "[]"
		}
		var sb strings.Builder
		for i, item := range val {
			if i > 0 {
				sb.WriteString("\n" + pad)
			}
			sb.WriteString("- ")
			itemStr := toYAMLString(item, indent+1)
			if strings.Contains(itemStr, "\n") {
				sb.WriteString("\n" + pad + "  " + strings.TrimPrefix(itemStr, "\n"))
			} else {
				sb.WriteString(itemStr)
			}
		}
		return sb.String()
	case map[string]any:
		if len(val) == 0 {
			return "{}"
		}
		var sb strings.Builder
		first := true
		for k, item := range val {
			if !first {
				sb.WriteString("\n" + pad)
			}
			first = false
			sb.WriteString(k + ": ")
			itemStr := toYAMLString(item, indent+1)
			if strings.Contains(itemStr, "\n") || strings.HasPrefix(itemStr, "-") {
				sb.WriteString("\n" + pad + "  " + itemStr)
			} else {
				sb.WriteString(itemStr)
			}
		}
		return sb.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}


// Text manipulation functions

// builtinIndent indents all lines of text by the specified number of spaces.
func builtinIndent(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("indent requires text and spaces arguments")
	}

	textStr, ok := args[0].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("indent: text must be a string")
	}

	spacesVal, ok := args[1].(*slop.IntValue)
	if !ok {
		return nil, fmt.Errorf("indent: spaces must be an integer")
	}

	pad := strings.Repeat(" ", int(spacesVal.Value))
	lines := strings.Split(textStr.Value, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = pad + line
		}
	}

	return slop.NewStringValue(strings.Join(lines, "\n")), nil
}

// builtinDedent removes common leading whitespace from all lines.
func builtinDedent(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("dedent requires text argument")
	}

	textStr, ok := args[0].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("dedent: text must be a string")
	}

	lines := strings.Split(textStr.Value, "\n")

	// Find minimum indent (ignoring empty lines)
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}

	if minIndent <= 0 {
		return slop.NewStringValue(textStr.Value), nil
	}

	// Remove common indent
	for i, line := range lines {
		if len(line) >= minIndent {
			lines[i] = line[minIndent:]
		}
	}

	return slop.NewStringValue(strings.Join(lines, "\n")), nil
}

// builtinWrap wraps text at the specified width.
func builtinWrap(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("wrap requires text and width arguments")
	}

	textStr, ok := args[0].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("wrap: text must be a string")
	}

	widthVal, ok := args[1].(*slop.IntValue)
	if !ok {
		return nil, fmt.Errorf("wrap: width must be an integer")
	}

	width := int(widthVal.Value)
	if width <= 0 {
		return slop.NewStringValue(textStr.Value), nil
	}

	words := strings.Fields(textStr.Value)
	if len(words) == 0 {
		return slop.NewStringValue(""), nil
	}

	var lines []string
	currentLine := words[0]

	for _, word := range words[1:] {
		if len(currentLine)+1+len(word) <= width {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	lines = append(lines, currentLine)

	return slop.NewStringValue(strings.Join(lines, "\n")), nil
}
