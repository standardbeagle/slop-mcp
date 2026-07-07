package main

import "testing"

func TestParseServeArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantPort int
		wantHelp bool
		wantErr  bool
	}{
		{name: "default stdio", args: nil},
		{name: "long port separate", args: []string{"--port", "8080"}, wantPort: 8080},
		{name: "long port equals", args: []string{"--port=3000"}, wantPort: 3000},
		{name: "short port", args: []string{"-p", "9000"}, wantPort: 9000},
		{name: "help", args: []string{"--help"}, wantHelp: true},
		{name: "missing port", args: []string{"--port"}, wantErr: true},
		{name: "empty equals port", args: []string{"--port="}, wantErr: true},
		{name: "invalid port", args: []string{"--port", "abc"}, wantErr: true},
		{name: "unknown option", args: []string{"--verbose"}, wantErr: true},
		{name: "positional garbage", args: []string{"config.kdl"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseServeArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got options %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseServeArgs: %v", err)
			}
			if got.port != tt.wantPort {
				t.Fatalf("port = %d, want %d", got.port, tt.wantPort)
			}
			if got.showHelp != tt.wantHelp {
				t.Fatalf("showHelp = %v, want %v", got.showHelp, tt.wantHelp)
			}
		})
	}
}
