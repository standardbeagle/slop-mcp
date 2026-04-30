package llm

import (
	"encoding/json"
	"testing"
)

func TestMessage_JSONRoundTrip(t *testing.T) {
	m := Message{Role: RoleUser, Content: "hi"}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got Message
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Role != RoleUser || got.Content != "hi" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestStopReason_Constants(t *testing.T) {
	if StopReasonEndTurn == "" || StopReasonToolUse == "" || StopReasonMaxTokens == "" {
		t.Fatal("stop-reason constants must be non-empty")
	}
}
