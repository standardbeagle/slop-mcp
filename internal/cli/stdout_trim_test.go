package cli

import (
	"testing"

	kdl "github.com/sblinch/kdl-go"
)

// TestStdoutTrimDefault verifies that trim defaults to true when unspecified and
// only disables when set to `trim false`. A plain bool field could not tell an
// absent trim node from an explicit false, silently disabling the default.
func TestStdoutTrimDefault(t *testing.T) {
	explicitFalse := "cli \"x\" {\n  command \"echo\"\n  stdout {\n    format \"json\"\n    trim false\n  }\n}\n"
	var f KDLFile
	if err := kdl.Unmarshal([]byte(explicitFalse), &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	st := f.Tools[0].Stdout
	if st == nil || st.Trim == nil {
		t.Fatal("trim should have parsed into a non-nil *bool")
	}
	if *st.Trim {
		t.Error("trim should be false")
	}
	if st.TrimEnabled() {
		t.Error("TrimEnabled should be false when trim false")
	}

	unspecified := "cli \"y\" {\n  command \"echo\"\n  stdout {\n    format \"json\"\n  }\n}\n"
	var f2 KDLFile
	if err := kdl.Unmarshal([]byte(unspecified), &f2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !f2.Tools[0].Stdout.TrimEnabled() {
		t.Error("unspecified trim should default to true")
	}

	// Nil StdoutConfig (no stdout block) also trims by default.
	if !(*StdoutConfig)(nil).TrimEnabled() {
		t.Error("nil StdoutConfig should default to trim=true")
	}
}
