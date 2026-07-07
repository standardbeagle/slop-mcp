package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/standardbeagle/slop-mcp/internal/builtins"
	"github.com/standardbeagle/slop-mcp/internal/cli"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/logging"
	"github.com/standardbeagle/slop-mcp/internal/overrides"
	"github.com/standardbeagle/slop-mcp/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthCheckInterval(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		want    string
		wantErr string
	}{
		{
			name: "nil config",
		},
		{
			name: "empty config",
			cfg:  config.NewConfig(),
		},
		{
			name: "disabled values",
			cfg: configWithHealthIntervals(map[string]string{
				"a": "",
				"b": "0",
			}),
		},
		{
			name: "single interval",
			cfg: configWithHealthIntervals(map[string]string{
				"a": "30s",
			}),
			want: "30s",
		},
		{
			name: "shortest interval wins",
			cfg: configWithHealthIntervals(map[string]string{
				"a": "1m",
				"b": "15s",
				"c": "30s",
			}),
			want: "15s",
		},
		{
			name: "invalid interval",
			cfg: configWithHealthIntervals(map[string]string{
				"a": "bad",
			}),
			wantErr: "invalid health_check_interval",
		},
		{
			name: "negative interval",
			cfg: configWithHealthIntervals(map[string]string{
				"a": "-1s",
			}),
			wantErr: "must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := healthCheckInterval(tt.cfg)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestServerStartRejectsInvalidHealthCheckInterval(t *testing.T) {
	s := newServerForStartTest(configWithHealthIntervals(map[string]string{
		"bad-mcp": "not-a-duration",
	}))

	err := s.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid health_check_interval")
	assert.Empty(t, s.registry.Status())
}

func TestOpenOverrideStoreHonorsXDGConfigHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	store, err := openOverrideStore(registry.New())
	require.NoError(t, err)

	err = store.SetCustom(overrides.ScopeUser, "xdg_tool", overrides.CustomTool{
		Description: "stored under xdg config home",
		Body:        "1",
	})
	require.NoError(t, err)
	require.NoError(t, store.Close())

	expected := filepath.Join(xdg, "slop-mcp", "memory", "_slop", overrides.BankCustomTools+".json")
	_, err = os.Stat(expected)
	require.NoError(t, err)
}

func configWithHealthIntervals(intervals map[string]string) *config.Config {
	cfg := config.NewConfig()
	for name, interval := range intervals {
		cfg.MCPs[name] = config.MCPConfig{
			Name:                name,
			Type:                "stdio",
			Command:             "example",
			HealthCheckInterval: interval,
		}
	}
	return cfg
}

func newServerForStartTest(cfg *config.Config) *Server {
	return &Server{
		registry:     registry.New(),
		cliRegistry:  cli.NewRegistry(),
		config:       cfg,
		logger:       logging.Default(),
		sessionStore: builtins.NewSessionStore(),
		memoryStore:  builtins.NewMemoryStore(),
	}
}
