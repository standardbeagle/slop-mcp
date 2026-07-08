package main

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
)

// fixedGetwd overrides the package getwd for a test and restores it after.
func fixedGetwd(t *testing.T, cwd string) {
	t.Helper()
	orig := getwd
	getwd = func() (string, error) { return cwd, nil }
	t.Cleanup(func() { getwd = orig })
}

func scopedName(cwd string) string {
	sum := sha256.Sum256([]byte(cwd))
	return "monitor-messages-" + hex.EncodeToString(sum[:8])
}

func TestMonitorMessagesPathHonorsXDGConfigHome(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	fixedGetwd(t, "/project/alpha")

	want := filepath.Join(configHome, "slop-mcp", scopedName("/project/alpha"))
	if got := monitorMessagesPath(); got != want {
		t.Fatalf("monitorMessagesPath() = %q, want %q", got, want)
	}
}

func TestMonitorMessagesPathFallsBackToHomeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	fixedGetwd(t, "/project/beta")

	want := filepath.Join(home, ".config", "slop-mcp", scopedName("/project/beta"))
	if got := monitorMessagesPath(); got != want {
		t.Fatalf("monitorMessagesPath() = %q, want %q", got, want)
	}
}

// Different projects (working directories) must resolve to different files.
func TestMonitorMessagesPathIsPerProject(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	fixedGetwd(t, "/project/alpha")
	a := monitorMessagesPath()
	fixedGetwd(t, "/project/gamma")
	b := monitorMessagesPath()
	if a == b {
		t.Fatalf("expected distinct paths per project, both = %q", a)
	}
}

func TestMonitorMessagesPathUnavailable(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")

	if got := monitorMessagesPath(); got != "" {
		t.Fatalf("monitorMessagesPath() = %q, want empty", got)
	}
}
