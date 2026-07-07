package main

import (
	"testing"
	"time"
)

func TestParseMonitorArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantFile    string
		wantInline  string
		wantHelp    bool
		wantTimeout time.Duration
		wantErr     bool
	}{
		{name: "watch messages only", args: nil},
		{name: "script file", args: []string{"watch.slop"}, wantFile: "watch.slop"},
		{name: "inline script", args: []string{"-e", `print("ok")`}, wantInline: `print("ok")`},
		{name: "timeout equals", args: []string{"--timeout=30s"}, wantTimeout: 30 * time.Second},
		{name: "timeout separate", args: []string{"--timeout", "2m"}, wantTimeout: 2 * time.Minute},
		{name: "help", args: []string{"-h"}, wantHelp: true},
		{name: "missing inline", args: []string{"-e"}, wantErr: true},
		{name: "missing timeout", args: []string{"--timeout"}, wantErr: true},
		{name: "bad timeout", args: []string{"--timeout=0"}, wantErr: true},
		{name: "unknown option", args: []string{"--verbose"}, wantErr: true},
		{name: "extra positional", args: []string{"a.slop", "b.slop"}, wantErr: true},
		{name: "file and inline", args: []string{"a.slop", "-e", "1"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMonitorArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got options %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMonitorArgs: %v", err)
			}
			if got.scriptFile != tt.wantFile {
				t.Fatalf("scriptFile = %q, want %q", got.scriptFile, tt.wantFile)
			}
			if got.inlineScript != tt.wantInline {
				t.Fatalf("inlineScript = %q, want %q", got.inlineScript, tt.wantInline)
			}
			if got.showHelp != tt.wantHelp {
				t.Fatalf("showHelp = %v, want %v", got.showHelp, tt.wantHelp)
			}
			if got.timeout != tt.wantTimeout {
				t.Fatalf("timeout = %v, want %v", got.timeout, tt.wantTimeout)
			}
		})
	}
}
