package registry

import (
	"encoding/json"
	"sort"
	"strings"
)

// PreflightParameters checks a call's parameters against the target tool's
// cached input schema BEFORE the call is dispatched, and returns a corrective
// error instead of nil when the call cannot succeed as written.
//
// Why this exists: the enriched "did you mean" diagnostics only ever ran on the
// reactive path (ExecuteTool, after the target server had already rejected the
// call), while execute_tool forwards raw JSON through ExecuteToolRawJSON to
// preserve integer precision — so in practice every parameter mistake cost a
// full round-trip and came back in whatever prose the target server chose. A
// 2026-07-27 audit of three machines found the same handful of shapes guessed
// wrong over and over against one server (a required `version` omitted, `id`
// sent as `taskId`, `paths` sent as `fileScope`), each costing a round-trip and
// often a blind retry of the identical shape.
//
// It is deliberately conservative, because a false rejection breaks a call that
// would have worked:
//
//   - Missing REQUIRED parameters are always reported. The schema states these
//     unambiguously; no target server accepts the call without them.
//   - Unknown parameters are reported ONLY when they closely resemble an
//     expected name (findSimilarParams' 50% threshold — i.e. we can name the
//     fix). An unrecognized name with no near match is forwarded untouched:
//     JSON Schema permits additional properties by default, and the target
//     server is the authority on whether it accepts them.
//   - No schema, no properties, or unparseable parameters means no opinion: the
//     call is forwarded and the server answers.
//
// The check never rewrites the caller's parameters. Silently renaming a field
// would make a mistyped call succeed against a shape the caller did not write.
func (r *Registry) PreflightParameters(mcpName, toolName string, params json.RawMessage) error {
	toolInfo := r.loadIndex().GetTool(mcpName, toolName)
	if toolInfo == nil || toolInfo.InputSchema == nil {
		return nil
	}

	expected := extractParamsFromSchema(toolInfo.InputSchema)
	if len(expected) == 0 {
		return nil
	}

	provided, ok := topLevelKeys(params)
	if !ok {
		return nil
	}

	expectedNorm := make(map[string]bool, len(expected))
	for _, e := range expected {
		expectedNorm[normalizeParam(e.Name)] = true
	}
	providedNorm := make(map[string]bool, len(provided))
	for _, p := range provided {
		providedNorm[normalizeParam(p)] = true
	}

	var unknown []string
	for _, p := range provided {
		if !expectedNorm[normalizeParam(p)] {
			unknown = append(unknown, p)
		}
	}
	sort.Strings(unknown)

	var missing []string
	for _, e := range expected {
		if e.Required && !providedNorm[normalizeParam(e.Name)] {
			missing = append(missing, e.Name)
		}
	}

	// Only the unknown names we can name a fix for are worth blocking on.
	similar := findSimilarParams(unknown, expected)
	matchUnknownToMissing(unknown, missing, similar)
	var misspelled []string
	for _, name := range unknown {
		if _, ok := similar[name]; ok {
			misspelled = append(misspelled, name)
		}
	}

	if len(missing) == 0 && len(misspelled) == 0 {
		return nil
	}

	return &InvalidParameterError{
		MCPName:         mcpName,
		ToolName:        toolName,
		OriginalError:   "rejected before dispatch — the call cannot succeed as written",
		ProvidedParams:  provided,
		ExpectedParams:  expected,
		SimilarParams:   similar,
		MissingRequired: missing,
		UnknownParams:   misspelled,
	}
}

// matchUnknownToMissing pairs a leftover unknown name with a required name the
// call did not supply, and records the pairing in similar.
//
// General similarity is too blunt for the commonest real mistake: a short
// canonical name qualified with the wrong prefix (`taskId` where the schema
// says `id`). Scored against every parameter, `taskid` vs `id` is only a
// one-third overlap and falls under findSimilarParams' bar. Scored against the
// names the call is actually MISSING, containment is decisive — the caller
// supplied a name nobody expects and omitted a required one it contains.
func matchUnknownToMissing(unknown, missing []string, similar map[string]string) {
	if len(missing) == 0 {
		return
	}
	claimed := make(map[string]bool, len(similar))
	for _, v := range similar {
		claimed[v] = true
	}

	for _, u := range unknown {
		if _, ok := similar[u]; ok {
			continue
		}
		uNorm := normalizeParam(u)
		best, bestScore := "", 0
		for _, m := range missing {
			if claimed[m] {
				continue
			}
			mNorm := normalizeParam(m)
			if !strings.Contains(uNorm, mNorm) {
				continue
			}
			// Prefer the longest containment: `taskId` contains both `id` and
			// (hypothetically) `taskid`, and the more specific name wins.
			if score := len(mNorm); score > bestScore {
				best, bestScore = m, score
			}
		}
		if best != "" {
			similar[u] = best
			claimed[best] = true
		}
	}
}

// EnrichParameterError re-frames a target server's error with the tool's schema
// when the message reads as a parameter complaint, and returns nil otherwise so
// ordinary tool failures keep their own wording. It is the raw-dispatch
// counterpart of the enrichment ExecuteTool applies inline.
func (r *Registry) EnrichParameterError(mcpName, toolName, serverError string, params json.RawMessage) error {
	if !isParameterError(serverError) {
		return nil
	}
	decoded := map[string]any{}
	if len(params) > 0 {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(params, &fields); err != nil {
			return nil
		}
		// Names are all the diagnostic needs; keeping values as raw bytes avoids
		// the float64 coercion this path exists to prevent.
		for name, raw := range fields {
			decoded[name] = string(raw)
		}
	}
	return r.createParameterError(mcpName, toolName, serverError, decoded)
}

// topLevelKeys reads only the field names of a JSON object, leaving every value
// as raw bytes. Decoding values here would defeat the reason execute_tool
// forwards raw JSON in the first place: coercing numbers through float64
// corrupts integers above 2^53.
func topLevelKeys(params json.RawMessage) ([]string, bool) {
	if len(params) == 0 {
		return nil, true
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(params, &fields); err != nil {
		return nil, false
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, true
}
