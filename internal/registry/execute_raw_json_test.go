package registry

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestCallToolParams_RawMessageArguments_PreserveLargeInts proves the wire
// representation ExecuteToolRawJSON hands to the SDK is lossless: a
// json.RawMessage placed in CallToolParams.Arguments is marshaled byte-for-byte,
// so integers above 2^53 survive (unlike a map[string]any round-trip, which
// coerces them to float64).
func TestCallToolParams_RawMessageArguments_PreserveLargeInts(t *testing.T) {
	raw := json.RawMessage(`{"lease_token":9007199254740993,"id":1234567890123456789}`)

	wire, err := json.Marshal(&mcp.CallToolParams{
		Name:      "task_workflow_step_advance",
		Arguments: raw,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got := string(wire)
	for _, want := range []string{"9007199254740993", "1234567890123456789"} {
		if !strings.Contains(got, want) {
			t.Errorf("large integer lost on the wire: want %q in %q", want, got)
		}
	}

	// Guard rail: the same payload through map[string]any DOES corrupt, which is
	// exactly the bug this path avoids.
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	lossy, _ := json.Marshal(m)
	if strings.Contains(string(lossy), "9007199254740993") {
		t.Skip("float64 coercion did not corrupt on this platform; precision guard moot")
	}
}
