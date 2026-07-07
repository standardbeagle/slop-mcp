package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/config"
)

func TestCurrentWorkingDirReturnsGetwdError(t *testing.T) {
	oldGetwd := getwd
	t.Cleanup(func() { getwd = oldGetwd })
	getwd = func() (string, error) {
		return "", errors.New("cwd unavailable")
	}

	got, err := currentWorkingDir()
	if err == nil {
		t.Fatalf("expected error, got cwd %q", got)
	}
	if !strings.Contains(err.Error(), "cwd unavailable") {
		t.Fatalf("error = %q, want wrapped getwd error", err.Error())
	}
}

func TestConfigPathForScopeReturnsGetwdError(t *testing.T) {
	oldGetwd := getwd
	t.Cleanup(func() { getwd = oldGetwd })
	getwd = func() (string, error) {
		return "", errors.New("cwd unavailable")
	}

	got, err := configPathForScope(config.ScopeProject)
	if err == nil {
		t.Fatalf("expected error, got path %q", got)
	}
	if !strings.Contains(err.Error(), "getting current directory") {
		t.Fatalf("error = %q, want current directory context", err.Error())
	}
}

func TestLoadMCPListJSONReturnsConfigLoadErrors(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, config.ProjectConfigFile), []byte(`mcp "broken" {`), 0644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	got, err := loadMCPListJSON(projectDir, false, true, false)
	if err == nil {
		t.Fatal("expected config load error")
	}
	if got != nil {
		t.Fatalf("expected nil result on error, got %#v", got)
	}
}

func TestLoadMCPListJSONMergesByPrecedence(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	userConfigDir := filepath.Join(tmp, "xdg")
	t.Setenv("XDG_CONFIG_HOME", userConfigDir)

	if err := os.MkdirAll(filepath.Join(userConfigDir, config.UserConfigDir), 0755); err != nil {
		t.Fatalf("mkdir user config dir: %v", err)
	}
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	userKDL := `mcp "shared" {
    command "user-cmd"
}
mcp "user-only" {
    command "user-only-cmd"
}
`
	projectKDL := `mcp "shared" {
    command "project-cmd"
}
mcp "project-only" {
    command "project-only-cmd"
}
`
	localKDL := `mcp "shared" {
    command "local-cmd"
}
mcp "local-only" {
    command "local-only-cmd"
}
`
	if err := os.WriteFile(config.UserConfigPath(), []byte(userKDL), 0644); err != nil {
		t.Fatalf("write user config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, config.ProjectConfigFile), []byte(projectKDL), 0644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, config.LocalConfigFile), []byte(localKDL), 0644); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	got, err := loadMCPListJSON(projectDir, true, true, true)
	if err != nil {
		t.Fatalf("loadMCPListJSON: %v", err)
	}

	if got["shared"].Command != "local-cmd" {
		t.Fatalf("expected local config to win for shared MCP, got %q", got["shared"].Command)
	}
	for name, want := range map[string]string{
		"user-only":    "user-only-cmd",
		"project-only": "project-only-cmd",
		"local-only":   "local-only-cmd",
	} {
		if got[name].Command != want {
			t.Fatalf("%s command = %q, want %q", name, got[name].Command, want)
		}
	}
}

func TestParseMCPPort(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{name: "minimum", raw: "1", want: 1},
		{name: "typical", raw: "8080", want: 8080},
		{name: "maximum", raw: "65535", want: 65535},
		{name: "empty", raw: "", wantErr: true},
		{name: "non numeric", raw: "abc", wantErr: true},
		{name: "trailing garbage", raw: "8080abc", wantErr: true},
		{name: "zero", raw: "0", wantErr: true},
		{name: "negative", raw: "-1", wantErr: true},
		{name: "too large", raw: "65536", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMCPPort(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got port %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMCPPort(%q): %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("parseMCPPort(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseMCPStatusArgs(t *testing.T) {
	opts, err := parseMCPStatusArgs([]string{"--port", "3000", "--json"})
	if err != nil {
		t.Fatalf("parseMCPStatusArgs: %v", err)
	}
	if opts.port != 3000 || !opts.outputJSON || opts.showHelp {
		t.Fatalf("unexpected options: %#v", opts)
	}

	opts, err = parseMCPStatusArgs([]string{"--port=4000"})
	if err != nil {
		t.Fatalf("parseMCPStatusArgs equals: %v", err)
	}
	if opts.port != 4000 {
		t.Fatalf("port = %d, want 4000", opts.port)
	}

	if _, err := parseMCPStatusArgs([]string{"--bogus"}); err == nil {
		t.Fatal("expected unknown option error")
	}
	if _, err := parseMCPStatusArgs([]string{"--port"}); err == nil {
		t.Fatal("expected missing port value error")
	}
	if _, err := parseMCPStatusArgs([]string{"server-name"}); err == nil {
		t.Fatal("expected positional argument error")
	}
}

func TestParseMCPMetadataArgs(t *testing.T) {
	opts, err := parseMCPMetadataArgs([]string{"--port", "3000", "--output", "mcps.json", "--mcp=figma", "--json"})
	if err != nil {
		t.Fatalf("parseMCPMetadataArgs: %v", err)
	}
	if opts.port != 3000 || opts.outputFile != "mcps.json" || opts.mcpName != "figma" || !opts.outputJSON || opts.showHelp {
		t.Fatalf("unexpected options: %#v", opts)
	}

	opts, err = parseMCPMetadataArgs([]string{"--port=4000", "--output=out.json", "--mcp", "github"})
	if err != nil {
		t.Fatalf("parseMCPMetadataArgs equals: %v", err)
	}
	if opts.port != 4000 || opts.outputFile != "out.json" || opts.mcpName != "github" {
		t.Fatalf("unexpected equals options: %#v", opts)
	}

	for _, args := range [][]string{
		{"--bogus"},
		{"--port"},
		{"--output"},
		{"--output="},
		{"--mcp"},
		{"--mcp="},
		{"positional"},
	} {
		if _, err := parseMCPMetadataArgs(args); err == nil {
			t.Fatalf("expected error for args %#v", args)
		}
	}
}

func TestSortedMCPNames(t *testing.T) {
	got := sortedMCPNames(map[string]config.MCPConfig{
		"zeta":  {},
		"alpha": {},
		"mid":   {},
	})
	want := []string{"alpha", "mid", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("sortedMCPNames length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortedMCPNames = %v, want %v", got, want)
		}
	}
}

func TestPrintMCPListSortsByName(t *testing.T) {
	cfg := &config.Config{MCPs: map[string]config.MCPConfig{
		"zeta":  {Name: "zeta", Type: "stdio", Command: "z"},
		"alpha": {Name: "alpha", Type: "http", URL: "https://example.com"},
		"mid":   {Name: "mid", Type: "stdio", Command: "m"},
	}}

	output := captureStdout(t, func() {
		printMCPList(cfg)
	})

	alpha := strings.Index(output, "  alpha:")
	mid := strings.Index(output, "  mid:")
	zeta := strings.Index(output, "  zeta:")
	if alpha < 0 || mid < 0 || zeta < 0 {
		t.Fatalf("missing expected MCPs in output:\n%s", output)
	}
	if !(alpha < mid && mid < zeta) {
		t.Fatalf("MCPs not sorted by name in output:\n%s", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = old
	})

	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close stdout pipe writer: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	return buf.String()
}

func TestWriteCLIOutputFileCreatesParents(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "nested", "metadata.json")

	if err := writeCLIOutputFile(outPath, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("writeCLIOutputFile: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != `{"ok":true}` {
		t.Fatalf("output = %q", string(data))
	}
}

func TestParseMCPAddArgs(t *testing.T) {
	opts, showHelp, err := parseMCPAddArgs([]string{
		"filesystem", "npx", "-y", "@anthropic/mcp-server-filesystem", "/tmp",
		"--env", "TOKEN=abc", "--header=Authorization: Bearer token", "--user",
	})
	if err != nil {
		t.Fatalf("parseMCPAddArgs stdio: %v", err)
	}
	if showHelp {
		t.Fatal("did not expect help")
	}
	if opts.name != "filesystem" || opts.command != "npx" || opts.transport != "stdio" || opts.scope != config.ScopeUser {
		t.Fatalf("unexpected stdio opts: %#v", opts)
	}
	wantArgs := []string{"-y", "@anthropic/mcp-server-filesystem", "/tmp"}
	if len(opts.cmdArgs) != len(wantArgs) {
		t.Fatalf("cmdArgs = %v, want %v", opts.cmdArgs, wantArgs)
	}
	for i := range wantArgs {
		if opts.cmdArgs[i] != wantArgs[i] {
			t.Fatalf("cmdArgs = %v, want %v", opts.cmdArgs, wantArgs)
		}
	}
	if opts.env["TOKEN"] != "abc" || opts.headers["Authorization"] != "Bearer token" {
		t.Fatalf("env/headers not parsed: %#v %#v", opts.env, opts.headers)
	}

	opts, showHelp, err = parseMCPAddArgs([]string{"api", "--transport=http", "--url=https://example.test/mcp"})
	if err != nil {
		t.Fatalf("parseMCPAddArgs http: %v", err)
	}
	if showHelp || opts.name != "api" || opts.transport != "http" || opts.url != "https://example.test/mcp" || opts.command != "" {
		t.Fatalf("unexpected http opts: %#v help=%v", opts, showHelp)
	}

	for _, args := range [][]string{
		{},
		{"api", "extra", "--transport=http", "--url=https://example.test/mcp"},
		{"api", "--transport=http"},
		{"api", "--transport=ftp", "--url=https://example.test/mcp"},
		{"api", "--transport=", "--url=https://example.test/mcp"},
		{"api", "npx", "--env", "=bad"},
		{"api", "npx", "--header", ": missing-name"},
		{"api", "npx", "--scope=global"},
	} {
		if _, _, err := parseMCPAddArgs(args); err == nil {
			t.Fatalf("expected error for args %#v", args)
		}
	}
}

func TestParseMCPAddJSONArgs(t *testing.T) {
	name, jsonStr, scope, showHelp, err := parseMCPAddJSONArgs([]string{"filesystem", `{"command":"npx"}`, "--user"})
	if err != nil {
		t.Fatalf("parseMCPAddJSONArgs: %v", err)
	}
	if name != "filesystem" || jsonStr != `{"command":"npx"}` || scope != config.ScopeUser || showHelp {
		t.Fatalf("got name=%q json=%q scope=%q help=%v", name, jsonStr, scope, showHelp)
	}

	name, jsonStr, scope, showHelp, err = parseMCPAddJSONArgs([]string{"--scope=local", "filesystem", `{"command":"npx"}`})
	if err != nil {
		t.Fatalf("parseMCPAddJSONArgs scope equals: %v", err)
	}
	if name != "filesystem" || jsonStr != `{"command":"npx"}` || scope != config.ScopeLocal || showHelp {
		t.Fatalf("got name=%q json=%q scope=%q help=%v", name, jsonStr, scope, showHelp)
	}

	for _, args := range [][]string{
		{},
		{"filesystem"},
		{"filesystem", `{"command":"npx"}`, "ignored"},
		{"--bogus", "filesystem", `{"command":"npx"}`},
		{"filesystem", `{"command":"npx"}`, "--scope"},
		{"filesystem", `{"command":"npx"}`, "--scope", "global"},
		{"filesystem", `{"command":"npx"}`, "--scope=global"},
	} {
		if _, _, _, _, err := parseMCPAddJSONArgs(args); err == nil {
			t.Fatalf("expected error for args %#v", args)
		}
	}
}

func TestParseMCPAddFromClaudeDesktopArgs(t *testing.T) {
	scope, names, showHelp, err := parseMCPAddFromClaudeDesktopArgs([]string{"filesystem", "github", "--scope=local"})
	if err != nil {
		t.Fatalf("parseMCPAddFromClaudeDesktopArgs: %v", err)
	}
	if scope != config.ScopeLocal || showHelp {
		t.Fatalf("got scope=%q help=%v", scope, showHelp)
	}
	wantNames := []string{"filesystem", "github"}
	if len(names) != len(wantNames) {
		t.Fatalf("names = %v, want %v", names, wantNames)
	}
	for i := range wantNames {
		if names[i] != wantNames[i] {
			t.Fatalf("names = %v, want %v", names, wantNames)
		}
	}

	scope, names, showHelp, err = parseMCPAddFromClaudeDesktopArgs([]string{"--user"})
	if err != nil {
		t.Fatalf("parseMCPAddFromClaudeDesktopArgs user: %v", err)
	}
	if scope != config.ScopeUser || len(names) != 0 || showHelp {
		t.Fatalf("got scope=%q names=%v help=%v", scope, names, showHelp)
	}

	for _, args := range [][]string{
		{"--bogus"},
		{"filesystem", "--scope"},
		{"filesystem", "--scope", "global"},
		{"filesystem", "--scope=global"},
	} {
		if _, _, _, err := parseMCPAddFromClaudeDesktopArgs(args); err == nil {
			t.Fatalf("expected error for args %#v", args)
		}
	}
}

func TestParseMCPAddFromClaudeCodeArgs(t *testing.T) {
	names, dryRun, showHelp, err := parseMCPAddFromClaudeCodeArgs([]string{"filesystem", "--dry-run"})
	if err != nil {
		t.Fatalf("parseMCPAddFromClaudeCodeArgs: %v", err)
	}
	if !dryRun || showHelp || len(names) != 1 || names[0] != "filesystem" {
		t.Fatalf("got names=%v dryRun=%v help=%v", names, dryRun, showHelp)
	}

	names, dryRun, showHelp, err = parseMCPAddFromClaudeCodeArgs([]string{"--help"})
	if err != nil {
		t.Fatalf("parseMCPAddFromClaudeCodeArgs help: %v", err)
	}
	if !showHelp || dryRun || len(names) != 0 {
		t.Fatalf("got names=%v dryRun=%v help=%v", names, dryRun, showHelp)
	}

	if _, _, _, err := parseMCPAddFromClaudeCodeArgs([]string{"--bogus"}); err == nil {
		t.Fatal("expected unknown option error")
	}
}

func TestParseMCPRemoveArgs(t *testing.T) {
	name, scope, showHelp, err := parseMCPRemoveArgs([]string{"filesystem", "--user"})
	if err != nil {
		t.Fatalf("parseMCPRemoveArgs: %v", err)
	}
	if name != "filesystem" || scope != config.ScopeUser || showHelp {
		t.Fatalf("got name=%q scope=%q help=%v", name, scope, showHelp)
	}

	name, scope, showHelp, err = parseMCPRemoveArgs([]string{"--scope=local", "filesystem"})
	if err != nil {
		t.Fatalf("parseMCPRemoveArgs scope equals: %v", err)
	}
	if name != "filesystem" || scope != config.ScopeLocal || showHelp {
		t.Fatalf("got name=%q scope=%q help=%v", name, scope, showHelp)
	}

	for _, args := range [][]string{
		{},
		{"one", "two"},
		{"--bogus", "filesystem"},
		{"filesystem", "--scope"},
		{"filesystem", "--scope", "global"},
		{"filesystem", "--scope=global"},
	} {
		if _, _, _, err := parseMCPRemoveArgs(args); err == nil {
			t.Fatalf("expected error for args %#v", args)
		}
	}
}

func TestParseMCPGetArgs(t *testing.T) {
	name, outputJSON, showHelp, err := parseMCPGetArgs([]string{"filesystem", "--json"})
	if err != nil {
		t.Fatalf("parseMCPGetArgs: %v", err)
	}
	if name != "filesystem" || !outputJSON || showHelp {
		t.Fatalf("got name=%q json=%v help=%v", name, outputJSON, showHelp)
	}

	if _, _, _, err := parseMCPGetArgs([]string{"--bogus", "filesystem"}); err == nil {
		t.Fatal("expected unknown option error")
	}
	if _, _, _, err := parseMCPGetArgs([]string{"one", "two"}); err == nil {
		t.Fatal("expected extra argument error")
	}
	if _, _, _, err := parseMCPGetArgs(nil); err == nil {
		t.Fatal("expected missing name error")
	}
}

func TestParseMCPListArgs(t *testing.T) {
	opts, err := parseMCPListArgs([]string{"--project", "--json"})
	if err != nil {
		t.Fatalf("parseMCPListArgs: %v", err)
	}
	if opts.showLocal || !opts.showProject || opts.showUser || !opts.outputJSON {
		t.Fatalf("unexpected options: %#v", opts)
	}

	opts, err = parseMCPListArgs([]string{"--project", "--all"})
	if err != nil {
		t.Fatalf("parseMCPListArgs --all: %v", err)
	}
	if !opts.showLocal || !opts.showProject || !opts.showUser {
		t.Fatalf("--all did not reset all scopes: %#v", opts)
	}

	if _, err := parseMCPListArgs([]string{"--bogus"}); err == nil {
		t.Fatal("expected unknown option error")
	}
	if _, err := parseMCPListArgs([]string{"filesystem"}); err == nil {
		t.Fatal("expected positional argument error")
	}
}

func TestParseMCPPathsArgs(t *testing.T) {
	showHelp, err := parseMCPPathsArgs([]string{"--help"})
	if err != nil {
		t.Fatalf("parseMCPPathsArgs: %v", err)
	}
	if !showHelp {
		t.Fatal("expected help")
	}
	if _, err := parseMCPPathsArgs([]string{"--json"}); err == nil {
		t.Fatal("expected unknown option error")
	}
}

func TestParseMCPDumpArgsRejectsUnknownAndMultipleScopes(t *testing.T) {
	if _, _, _, err := parseMCPDumpArgs([]string{"--bogus"}); err == nil {
		t.Fatal("expected unknown option error")
	}
	if _, _, _, err := parseMCPDumpArgs([]string{"--local", "--project"}); err == nil {
		t.Fatal("expected multiple scope error")
	}

	scope, outputJSON, showHelp, err := parseMCPDumpArgs([]string{"--project", "--json"})
	if err != nil {
		t.Fatalf("parseMCPDumpArgs: %v", err)
	}
	if scope != "project" || !outputJSON || showHelp {
		t.Fatalf("got scope=%q json=%v help=%v", scope, outputJSON, showHelp)
	}
}

func TestBuildMCPDumpJSONAllScopesIsSingleJSONObject(t *testing.T) {
	tmp := t.TempDir()
	localPath := filepath.Join(tmp, "local.kdl")
	if err := os.WriteFile(localPath, []byte(`mcp "local-one" {
    command "local-cmd"
}
`), 0644); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	paths := map[string]string{
		"local":          localPath,
		"project":        filepath.Join(tmp, `missing"project.kdl`),
		"user":           filepath.Join(tmp, "missing-user.kdl"),
		"claude_desktop": filepath.Join(tmp, "missing-desktop.json"),
		"claude_code":    filepath.Join(tmp, "missing-code.json"),
	}

	output, err := buildMCPDumpJSON(paths, "")
	if err != nil {
		t.Fatalf("buildMCPDumpJSON: %v", err)
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("output is not one JSON object: %v\n%s", err, data)
	}
	for _, name := range mcpDumpScopes {
		if _, ok := decoded[name]; !ok {
			t.Fatalf("missing %q in aggregate output: %s", name, data)
		}
	}

	var local map[string]config.MCPConfig
	if err := json.Unmarshal(decoded["local"], &local); err != nil {
		t.Fatalf("local output is not MCP config object: %v", err)
	}
	if len(local) != 1 || local["local-one"].Command != "local-cmd" {
		t.Fatalf("local output = %#v, want local-one", local)
	}

	var project map[string]string
	if err := json.Unmarshal(decoded["project"], &project); err != nil {
		t.Fatalf("project error output is not JSON object: %v", err)
	}
	if project["error"] == "" {
		t.Fatalf("project error missing in %s", decoded["project"])
	}
}

func TestLoadMCPDumpJSONRawFallbackEscapesAsJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.kdl")
	raw := "not kdl with \"quote\"\n"
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatalf("write broken config: %v", err)
	}

	output, err := loadMCPDumpJSON("project", path)
	if err != nil {
		t.Fatalf("loadMCPDumpJSON: %v", err)
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("raw fallback is not JSON-marshalable: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("raw fallback is not JSON object: %v", err)
	}
	if decoded["raw"] != raw {
		t.Fatalf("raw = %q, want %q", decoded["raw"], raw)
	}
}
