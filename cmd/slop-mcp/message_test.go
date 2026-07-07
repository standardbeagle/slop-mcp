package main

import (
	"path/filepath"
	"testing"
)

func TestMonitorMessagesPathHonorsXDGConfigHome(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	want := filepath.Join(configHome, "slop-mcp", "monitor-messages")
	if got := monitorMessagesPath(); got != want {
		t.Fatalf("monitorMessagesPath() = %q, want %q", got, want)
	}
}

func TestMonitorMessagesPathFallsBackToHomeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	want := filepath.Join(home, ".config", "slop-mcp", "monitor-messages")
	if got := monitorMessagesPath(); got != want {
		t.Fatalf("monitorMessagesPath() = %q, want %q", got, want)
	}
}

func TestMonitorMessagesPathUnavailable(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")

	if got := monitorMessagesPath(); got != "" {
		t.Fatalf("monitorMessagesPath() = %q, want empty", got)
	}
}
