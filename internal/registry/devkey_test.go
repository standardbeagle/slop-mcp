package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevkeyBinary_HonorsOverride(t *testing.T) {
	// Agents are routinely launched from non-login shells whose PATH omits
	// ~/.local/bin, so the override is the supported escape.
	dir := t.TempDir()
	fake := filepath.Join(dir, "devkey")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SLOP_DEVKEY_BINARY", fake)

	got, err := devkeyBinary()
	if err != nil {
		t.Fatalf("expected override to resolve, got %v", err)
	}
	if got != fake {
		t.Fatalf("expected %q, got %q", fake, got)
	}
}

func TestDevkeyBinary_OverrideMustExist(t *testing.T) {
	// A typo in the override must fail loudly here, not as an opaque connection
	// error after the transport is already being built.
	t.Setenv("SLOP_DEVKEY_BINARY", filepath.Join(t.TempDir(), "absent"))

	if _, err := devkeyBinary(); err == nil {
		t.Fatal("expected an error for a nonexistent override")
	}
}

func TestDevkeyBinary_MissingFromPathIsActionable(t *testing.T) {
	t.Setenv("SLOP_DEVKEY_BINARY", "")
	t.Setenv("PATH", t.TempDir())

	_, err := devkeyBinary()
	if err == nil {
		t.Fatal("expected an error when devkey is not on PATH")
	}
	// The message has to name the fix; "executable file not found" alone sends
	// the reader looking for a broken MCP server instead of a missing broker.
	for _, want := range []string{"devkey", "SLOP_DEVKEY_BINARY"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}
