package main

import (
	"testing"
	"time"
)

func TestParseRunArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantFile   string
		wantInline string
		wantJSON   bool
		wantHelp   bool
		wantTTL    time.Duration
		wantErr    bool
	}{
		{
			name:     "script file",
			args:     []string{"script.slop"},
			wantFile: "script.slop",
			wantTTL:  5 * time.Minute,
		},
		{
			name:       "inline script",
			args:       []string{"-e", "emit 1"},
			wantInline: "emit 1",
			wantTTL:    5 * time.Minute,
		},
		{
			name:       "json output",
			args:       []string{"-e", "emit 1", "--json"},
			wantInline: "emit 1",
			wantJSON:   true,
			wantTTL:    5 * time.Minute,
		},
		{
			name:       "timeout equals",
			args:       []string{"-e", "emit 1", "--timeout=30s"},
			wantInline: "emit 1",
			wantTTL:    30 * time.Second,
		},
		{
			name:       "timeout separate value",
			args:       []string{"-e", "emit 1", "--timeout", "2m"},
			wantInline: "emit 1",
			wantTTL:    2 * time.Minute,
		},
		{
			name:     "help without script",
			args:     []string{"--help"},
			wantHelp: true,
			wantTTL:  5 * time.Minute,
		},
		{
			name:    "missing script",
			wantTTL: 5 * time.Minute,
			wantErr: true,
		},
		{
			name:    "missing inline value",
			args:    []string{"-e"},
			wantTTL: 5 * time.Minute,
			wantErr: true,
		},
		{
			name:    "missing timeout value",
			args:    []string{"-e", "emit 1", "--timeout"},
			wantTTL: 5 * time.Minute,
			wantErr: true,
		},
		{
			name:    "invalid timeout",
			args:    []string{"-e", "emit 1", "--timeout=0"},
			wantTTL: 5 * time.Minute,
			wantErr: true,
		},
		{
			name:    "unknown option",
			args:    []string{"-e", "emit 1", "--verbose"},
			wantTTL: 5 * time.Minute,
			wantErr: true,
		},
		{
			name:    "extra positional script",
			args:    []string{"one.slop", "two.slop"},
			wantTTL: 5 * time.Minute,
			wantErr: true,
		},
		{
			name:    "file and inline script",
			args:    []string{"script.slop", "-e", "emit 1"},
			wantTTL: 5 * time.Minute,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseRunArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseRunArgs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.scriptFile != tt.wantFile {
				t.Fatalf("scriptFile = %q, want %q", got.scriptFile, tt.wantFile)
			}
			if got.inlineScript != tt.wantInline {
				t.Fatalf("inlineScript = %q, want %q", got.inlineScript, tt.wantInline)
			}
			if got.outputJSON != tt.wantJSON {
				t.Fatalf("outputJSON = %v, want %v", got.outputJSON, tt.wantJSON)
			}
			if got.showHelp != tt.wantHelp {
				t.Fatalf("showHelp = %v, want %v", got.showHelp, tt.wantHelp)
			}
			if got.timeout != tt.wantTTL {
				t.Fatalf("timeout = %v, want %v", got.timeout, tt.wantTTL)
			}
		})
	}
}
