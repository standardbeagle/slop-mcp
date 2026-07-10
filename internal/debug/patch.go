package debug

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type Patch struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value,omitempty"`
}

func ApplyPatches(input []byte, patches []Patch, validate func([]byte) error) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("decode checkpoint: %w", err)
	}
	for i, patch := range patches {
		var value any
		if patch.Op != "remove" {
			if len(patch.Value) == 0 {
				return nil, fmt.Errorf("patch %d: value is required", i)
			}
			vdec := json.NewDecoder(bytes.NewReader(patch.Value))
			vdec.UseNumber()
			if err := vdec.Decode(&value); err != nil {
				return nil, fmt.Errorf("patch %d value: %w", i, err)
			}
		}
		parts, err := pointerParts(patch.Path)
		if err != nil {
			return nil, fmt.Errorf("patch %d: %w", i, err)
		}
		if len(parts) == 0 {
			return nil, fmt.Errorf("patch %d: root mutation is not supported", i)
		}
		root, err = applyAt(root, parts, patch.Op, value)
		if err != nil {
			return nil, fmt.Errorf("patch %d: %w", i, err)
		}
	}
	out, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	if validate != nil {
		if err := validate(out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func pointerParts(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("JSON pointer must start with /")
	}
	raw := strings.Split(path[1:], "/")
	for i, part := range raw {
		for j := 0; j < len(part); j++ {
			if part[j] == '~' && (j+1 >= len(part) || (part[j+1] != '0' && part[j+1] != '1')) {
				return nil, fmt.Errorf("invalid JSON pointer escape in %q", part)
			}
		}
		raw[i] = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
	}
	return raw, nil
}

func applyAt(parent any, parts []string, op string, value any) (any, error) {
	key := parts[0]
	if len(parts) > 1 {
		switch node := parent.(type) {
		case map[string]any:
			child, ok := node[key]
			if !ok {
				return nil, fmt.Errorf("path component %q not found", key)
			}
			updated, err := applyAt(child, parts[1:], op, value)
			if err != nil {
				return nil, err
			}
			node[key] = updated
			return node, nil
		case []any:
			idx, err := arrayIndex(key, len(node), false)
			if err != nil {
				return nil, err
			}
			updated, err := applyAt(node[idx], parts[1:], op, value)
			if err != nil {
				return nil, err
			}
			node[idx] = updated
			return node, nil
		default:
			return nil, fmt.Errorf("path component %q has scalar parent", key)
		}
	}
	switch node := parent.(type) {
	case map[string]any:
		old, exists := node[key]
		switch op {
		case "test":
			if !exists || !reflect.DeepEqual(old, value) {
				return nil, fmt.Errorf("test failed at %q", key)
			}
		case "add":
			node[key] = value
		case "replace":
			if !exists {
				return nil, fmt.Errorf("path %q not found", key)
			}
			node[key] = value
		case "remove":
			if !exists {
				return nil, fmt.Errorf("path %q not found", key)
			}
			delete(node, key)
		default:
			return nil, fmt.Errorf("unsupported operation %q", op)
		}
		return node, nil
	case []any:
		if op == "add" && key == "-" {
			return append(node, value), nil
		}
		idx, err := arrayIndex(key, len(node), op == "add")
		if err != nil {
			return nil, err
		}
		switch op {
		case "test":
			if idx >= len(node) || !reflect.DeepEqual(node[idx], value) {
				return nil, fmt.Errorf("test failed at %q", key)
			}
		case "replace":
			if idx >= len(node) {
				return nil, fmt.Errorf("index out of range")
			}
			node[idx] = value
		case "remove":
			if idx >= len(node) {
				return nil, fmt.Errorf("index out of range")
			}
			return append(node[:idx], node[idx+1:]...), nil
		case "add":
			if idx == len(node) {
				return append(node, value), nil
			}
			node = append(node, nil)
			copy(node[idx+1:], node[idx:])
			node[idx] = value
		default:
			return nil, fmt.Errorf("unsupported operation %q", op)
		}
		return node, nil
	default:
		return nil, fmt.Errorf("target has scalar parent")
	}
}
func arrayIndex(raw string, length int, allowEnd bool) (int, error) {
	i, err := strconv.Atoi(raw)
	if err != nil || i < 0 || i > length || (!allowEnd && i == length) {
		return 0, fmt.Errorf("invalid array index %q", raw)
	}
	return i, nil
}

// Inspect returns a compact checkpoint summary or the explicitly requested
// JSON-pointer values. It avoids returning the full execution state by default.
func Inspect(input []byte, paths []string) (map[string]any, error) {
	var root any
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		object, ok := root.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("checkpoint root is not an object")
		}
		out := map[string]any{}
		for _, key := range []string{"version", "position", "reason", "failure", "breakpoints"} {
			if value, exists := object[key]; exists {
				out[key] = value
			}
		}
		return out, nil
	}
	out := make(map[string]any, len(paths))
	for _, path := range paths {
		parts, err := pointerParts(path)
		if err != nil {
			return nil, err
		}
		current := root
		for _, part := range parts {
			switch node := current.(type) {
			case map[string]any:
				var ok bool
				current, ok = node[part]
				if !ok {
					return nil, fmt.Errorf("path %q not found", path)
				}
			case []any:
				idx, err := arrayIndex(part, len(node), false)
				if err != nil {
					return nil, err
				}
				current = node[idx]
			default:
				return nil, fmt.Errorf("path %q has scalar component", path)
			}
		}
		out[path] = current
	}
	return out, nil
}
