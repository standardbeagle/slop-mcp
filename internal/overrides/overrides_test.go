// internal/overrides/overrides_test.go
package overrides

import (
	"encoding/json"
	"testing"
)

func TestOverrideEntry_JSONRoundTrip(t *testing.T) {
	e := OverrideEntry{
		Description: "short",
		Params:      map[string]string{"q": "query"},
		SourceHash:  "abc123",
		Scope:       ScopeUser,
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var got OverrideEntry
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Description != "short" || got.SourceHash != "abc123" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestBankNames(t *testing.T) {
	if BankOverrides != "_slop.overrides" {
		t.Errorf("BankOverrides = %q, want _slop.overrides", BankOverrides)
	}
	if BankCustomTools != "_slop.tools" {
		t.Errorf("BankCustomTools = %q, want _slop.tools", BankCustomTools)
	}
	if !IsReservedBank("_slop.overrides") || !IsReservedBank("_slop.anything") {
		t.Error("IsReservedBank should match _slop. prefix")
	}
	if IsReservedBank("user_bank") {
		t.Error("IsReservedBank should not match arbitrary names")
	}
}
