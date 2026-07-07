package auth

import (
	"path/filepath"
	"testing"
)

func TestNewTokenStoreHonorsXDGConfigHome(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	store := NewTokenStore()

	want := filepath.Join(configHome, "slop-mcp", "auth.json")
	if store.Path() != want {
		t.Fatalf("NewTokenStore path = %q, want %q", store.Path(), want)
	}
}

func TestNewTokenStoreFallsBackToHomeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	store := NewTokenStore()

	want := filepath.Join(home, ".config", "slop-mcp", "auth.json")
	if store.Path() != want {
		t.Fatalf("NewTokenStore path = %q, want %q", store.Path(), want)
	}
}

func TestNewTokenStorePathUnavailable(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")

	store := NewTokenStore()

	if store.Path() != "" {
		t.Fatalf("NewTokenStore path = %q, want empty", store.Path())
	}
}
