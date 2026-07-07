package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withFailingGetwd(t *testing.T) {
	t.Helper()
	oldGetwd := getwd
	getwd = func() (string, error) {
		return "", errors.New("cwd unavailable")
	}
	t.Cleanup(func() {
		getwd = oldGetwd
	})
}

func TestParseFlagsBooleanDoesNotConsumePositional(t *testing.T) {
	positional, flags, err := parseSearchFlags([]string{"needle", "--value", "literal"})
	if err != nil {
		t.Fatalf("parseSearchFlags: %v", err)
	}
	if len(positional) != 2 || positional[0] != "needle" || positional[1] != "literal" {
		t.Fatalf("positional = %#v, want [needle literal]", positional)
	}
	if flags["value"] != "true" {
		t.Fatalf("value flag = %q, want true", flags["value"])
	}
}

func TestParseFlagsRejectsUnknownFlag(t *testing.T) {
	_, _, err := parseReadFlags([]string{"session", "--verbose", "--typo"})
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
}

func TestParseFlagsValueWithDashRequiresEqualsForm(t *testing.T) {
	positional, flags, err := parseReadFlags([]string{"session", "missing", "--default=-1"})
	if err != nil {
		t.Fatalf("parseReadFlags: %v", err)
	}
	if len(positional) != 2 || positional[0] != "session" || positional[1] != "missing" {
		t.Fatalf("positional = %#v, want [session missing]", positional)
	}
	if flags["default"] != "-1" {
		t.Fatalf("default flag = %q, want -1", flags["default"])
	}
}

func TestParseFlagsRejectsMissingValue(t *testing.T) {
	_, _, err := parseWriteFlags([]string{"session", "key", "value", "--ttl"})
	if err == nil {
		t.Fatal("expected missing value error")
	}
}

func TestParseFlagsRejectsInvalidBooleanValue(t *testing.T) {
	_, _, err := parseQueryFlags([]string{"session", ".", "--raw=maybe"})
	if err == nil {
		t.Fatal("expected invalid boolean value error")
	}
}

func TestParseTTL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int64
		wantErr bool
	}{
		{name: "positive", raw: "3600", want: 3600},
		{name: "one", raw: "1", want: 1},
		{name: "empty", raw: "", wantErr: true},
		{name: "non numeric", raw: "abc", wantErr: true},
		{name: "trailing garbage", raw: "30s", wantErr: true},
		{name: "zero", raw: "0", wantErr: true},
		{name: "negative", raw: "-5", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTTL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got ttl %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTTL(%q): %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("parseTTL(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestApplyFilterRejectsInvalidArrayIndex(t *testing.T) {
	data := []any{"first", "second"}

	got, err := applyFilter(data, ".[abc]")
	if err == nil {
		t.Fatalf("expected invalid index error, got %#v", got)
	}
}

func TestApplyFilterArrayIndex(t *testing.T) {
	data := map[string]any{
		"items": []any{"first", "second"},
	}

	got, err := applyFilter(data, ".items.[1]")
	if err != nil {
		t.Fatalf("applyFilter: %v", err)
	}
	if got != "second" {
		t.Fatalf("got %#v, want second", got)
	}
}

func TestValidateMemoryScope(t *testing.T) {
	tests := []struct {
		name     string
		scope    string
		allowAll bool
		wantErr  bool
	}{
		{name: "auto", scope: ""},
		{name: "user", scope: "user"},
		{name: "project", scope: "project"},
		{name: "all allowed", scope: "all", allowAll: true},
		{name: "all rejected for single bank commands", scope: "all", wantErr: true},
		{name: "typo rejected", scope: "usr", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMemoryScope(tt.scope, tt.allowAll)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetProjectMemoryDirReturnsGetwdError(t *testing.T) {
	withFailingGetwd(t)

	got, err := getProjectMemoryDir()
	if err == nil {
		t.Fatalf("expected error, got dir %q", got)
	}
	if !strings.Contains(err.Error(), "getting current directory") {
		t.Fatalf("error = %q, want current directory context", err.Error())
	}
}

func TestGetBankPathReturnsGetwdErrorForProjectScope(t *testing.T) {
	withFailingGetwd(t)

	got, err := getBankPath("session", "project")
	if err == nil {
		t.Fatalf("expected error, got path %q", got)
	}
	if !strings.Contains(err.Error(), "cwd unavailable") {
		t.Fatalf("error = %q, want wrapped getwd error", err.Error())
	}
}

func TestCmdWriteReturnsPathErrorWhenProjectAutoDetectionFails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	withFailingGetwd(t)

	err := cmdWrite([]string{"session", "key", `"value"`})
	if err == nil {
		t.Fatal("expected path error")
	}
	if !strings.Contains(err.Error(), "cwd unavailable") {
		t.Fatalf("error = %q, want wrapped getwd error", err.Error())
	}
}

func TestParseDefaultValue(t *testing.T) {
	got, err := parseDefaultValue(`{"ok":true}`)
	if err != nil {
		t.Fatalf("parseDefaultValue: %v", err)
	}
	obj, ok := got.(map[string]any)
	if !ok || obj["ok"] != true {
		t.Fatalf("default value = %#v, want object with ok=true", got)
	}

	if _, err := parseDefaultValue(`{bad json}`); err == nil {
		t.Fatal("expected invalid default JSON error")
	}
}

func TestCmdReadRejectsInvalidDefaultJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := cmdRead([]string{"missing-bank", "missing-key", "--scope", "user", "--default", "{bad json}"})
	if err == nil {
		t.Fatal("expected invalid default JSON error")
	}
}

func TestCmdListRejectsAllScopeForSpecificBank(t *testing.T) {
	err := cmdList([]string{"session", "--scope", "all"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListBankInfosReturnsErrorForCorruptBank(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0644); err != nil {
		t.Fatalf("write corrupt bank: %v", err)
	}

	infos, err := listBankInfos(dir, "user")
	if err == nil {
		t.Fatalf("expected corrupt bank error, got infos %#v", infos)
	}
}

func TestListBankInfosMissingDirectoryIsEmpty(t *testing.T) {
	infos, err := listBankInfos(filepath.Join(t.TempDir(), "missing"), "user")
	if err != nil {
		t.Fatalf("listBankInfos missing dir: %v", err)
	}
	if len(infos) != 0 {
		t.Fatalf("infos = %#v, want empty", infos)
	}
}

func TestSearchBankFilesReturnsErrorForCorruptBank(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0644); err != nil {
		t.Fatalf("write corrupt bank: %v", err)
	}

	err := searchBankFiles(dir, "user", func(path, name, scope string) error {
		bank, err := loadBank(path)
		if err != nil {
			return fmt.Errorf("loading bank %q: %w", name, err)
		}
		if bank == nil {
			t.Fatalf("expected corrupt bank to fail before nil")
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected corrupt bank error")
	}
}

func TestSearchBankFilesMissingDirectoryIsEmpty(t *testing.T) {
	called := false
	err := searchBankFiles(filepath.Join(t.TempDir(), "missing"), "user", func(path, name, scope string) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("searchBankFiles missing dir: %v", err)
	}
	if called {
		t.Fatal("search callback should not be called for missing directory")
	}
}

func TestSearchBankFilesPropagatesCallbackError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bank.json"), []byte(`{"_meta":{"version":1},"entries":{}}`), 0644); err != nil {
		t.Fatalf("write bank: %v", err)
	}

	err := searchBankFiles(dir, "user", func(path, name, scope string) error {
		return fmt.Errorf("callback failed")
	})
	if err == nil {
		t.Fatal("expected callback error")
	}
}
