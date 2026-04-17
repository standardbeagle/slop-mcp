package overrides

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// canonicalJSON produces a deterministic JSON encoding with sorted map keys
// at every level and no extra whitespace. Arrays preserve their input order.
func canonicalJSON(v any) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := encodeCanonical(buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeCanonical(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
	case map[string]any:
		return encodeMap(buf, x)
	case map[string]string:
		m := make(map[string]any, len(x))
		for k, s := range x {
			m[k] = s
		}
		return encodeMap(buf, m)
	case []any:
		buf.WriteByte('[')
		for i, el := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeCanonical(buf, el); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	default:
		// Fall through to stdlib for scalars and anything else (including []int).
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}

func encodeMap(buf *bytes.Buffer, m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		if err := encodeCanonical(buf, m[k]); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

// ComputeHash returns the first 16 hex characters of
// SHA-256(description + "\n" + canonical_json(params)).
func ComputeHash(description string, params map[string]string) string {
	pb, err := canonicalJSON(params)
	if err != nil {
		// Fall back to fmt.Sprintf so hashing is always total.
		pb = []byte(fmt.Sprintf("%v", params))
	}
	h := sha256.New()
	h.Write([]byte(description))
	h.Write([]byte("\n"))
	h.Write(pb)
	return hex.EncodeToString(h.Sum(nil))[:16]
}
