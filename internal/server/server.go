package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/builtins"
	"github.com/standardbeagle/slop-mcp/internal/cli"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/logging"
	"github.com/standardbeagle/slop-mcp/internal/overrides"
	"github.com/standardbeagle/slop-mcp/internal/registry"
)

const (
	serverName    = "slop-mcp"
	serverVersion = "0.14.5"
)

// Keep the downstream MCP client Implementation version in sync with the
// server version instead of a stale hardcoded constant.
func init() {
	registry.ClientVersion = serverVersion
}

// Server is the slop-mcp server.
type Server struct {
	mcpServer     *mcp.Server
	registry      *registry.Registry
	cliRegistry   *cli.Registry
	config        *config.Config
	logger        logging.Logger
	sessionStore  *builtins.SessionStore
	memoryStore   *builtins.MemoryStore
	overrideStore *overrides.Store
}

// openOverrideStore builds and opens the overrides store using standard config paths,
// then wires the registry override provider.
func openOverrideStore(reg *registry.Registry) (*overrides.Store, error) {
	userConfigPath := config.UserConfigPath()
	if userConfigPath == "" {
		return nil, fmt.Errorf("user config path unavailable")
	}
	opts := overrides.StoreOptions{
		UserRoot: filepath.Join(filepath.Dir(userConfigPath), "memory", "_slop"),
	}
	if cwd, err := os.Getwd(); err == nil {
		if root, err := overrides.FindRepoRoot(cwd); err == nil {
			opts.ProjectRoot = filepath.Join(root, ".slop-mcp", "memory", "_slop")
			opts.LocalRoot = filepath.Join(root, ".slop-mcp", "memory.local", "_slop")
		}
	}
	store, err := overrides.OpenStore(opts)
	if err != nil {
		return nil, fmt.Errorf("overrides store: %w", err)
	}
	reg.SetOverrideProvider(&storeOverrideProvider{store: store})
	return store, nil
}

// New creates a new Server with the given config.
func New(ctx context.Context, mcps []config.MCPConfig) (*Server, error) {
	reg := registry.New()
	store, err := openOverrideStore(reg)
	if err != nil {
		return nil, err
	}
	s := &Server{
		registry:      reg,
		cliRegistry:   cli.NewRegistry(),
		config:        config.NewConfig(),
		logger:        logging.Default(),
		sessionStore:  builtins.NewSessionStore(),
		memoryStore:   builtins.NewMemoryStore(),
		overrideStore: store,
	}

	// Create MCP server
	s.mcpServer = mcp.NewServer(
		&mcp.Implementation{
			Name:    serverName,
			Version: serverVersion,
		},
		&mcp.ServerOptions{
			Capabilities: &mcp.ServerCapabilities{
				Tools: &mcp.ToolCapabilities{},
			},
		},
	)

	// Register all tools
	s.registerTools()

	// Connect to provided MCPs
	for _, mcpCfg := range mcps {
		if err := s.registry.Connect(ctx, mcpCfg); err != nil {
			s.Close()
			return nil, fmt.Errorf("failed to connect to MCP %s: %w", mcpCfg.Name, err)
		}
	}

	return s, nil
}

// NewFromConfig creates a new Server from a config struct.
func NewFromConfig(cfg *config.Config) (*Server, error) {
	reg := registry.New()
	store, err := openOverrideStore(reg)
	if err != nil {
		return nil, err
	}
	s := &Server{
		registry:      reg,
		cliRegistry:   cli.NewRegistry(),
		config:        cfg,
		logger:        logging.Default(),
		sessionStore:  builtins.NewSessionStore(),
		memoryStore:   builtins.NewMemoryStore(),
		overrideStore: store,
	}

	// Create MCP server
	s.mcpServer = mcp.NewServer(
		&mcp.Implementation{
			Name:    serverName,
			Version: serverVersion,
		},
		&mcp.ServerOptions{
			Capabilities: &mcp.ServerCapabilities{
				Tools: &mcp.ToolCapabilities{},
			},
		},
	)

	// Register all tools
	s.registerTools()

	return s, nil
}

// Start connects to all configured MCPs in the background.
// This is non-blocking - the server will be ready immediately while
// MCP connections are established asynchronously.
// Cached MCPs are loaded from disk immediately and lazy-connect on demand.
func (s *Server) Start(ctx context.Context) error {
	interval, err := healthCheckInterval(s.config)
	if err != nil {
		return err
	}

	// Register all configured MCPs first (so status shows them immediately)
	for _, cfg := range s.config.MCPs {
		s.registry.SetConfigured(cfg)
	}

	// Load CLI tools from configured directories
	s.loadCLITools()

	// Load cached tool metadata (non-dynamic MCPs with valid cache)
	// Cached MCPs skip eager connection and lazy-connect on first execute_tool
	cached := s.registry.LoadCache(s.config)
	if cached > 0 {
		s.logger.Info("loaded MCPs from cache", "count", cached)
	}

	if interval != "" {
		if err := s.registry.StartBackgroundHealthCheck(interval); err != nil {
			return err
		}
	}

	// Connect to MCPs in background to avoid blocking server startup
	// Cached MCPs are skipped (ConnectFromConfig checks for StateCached)
	go func() {
		if err := s.registry.ConnectFromConfig(ctx, s.config); err != nil {
			// Debug level: individual connection errors stored in registry state
			s.logger.Debug("error connecting to MCPs", "error", err)
		}
	}()
	return nil
}

func healthCheckInterval(cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", nil
	}

	var shortest time.Duration
	for name, mcpCfg := range cfg.MCPs {
		raw := mcpCfg.HealthCheckInterval
		if raw == "" || raw == "0" {
			continue
		}
		interval, err := time.ParseDuration(raw)
		if err != nil {
			return "", fmt.Errorf("invalid health_check_interval for MCP %s: %w", name, err)
		}
		if interval <= 0 {
			return "", fmt.Errorf("invalid health_check_interval for MCP %s: must be positive", name)
		}
		if shortest == 0 || interval < shortest {
			shortest = interval
		}
	}
	if shortest == 0 {
		return "", nil
	}
	return shortest.String(), nil
}

// loadCLITools loads CLI tool definitions from standard directories.
func (s *Server) loadCLITools() {
	// User-level CLI tools: $XDG_CONFIG_HOME/slop-mcp/cli/ or ~/.config/slop-mcp/cli/
	if userConfigDir := config.UserConfigDirPath(); userConfigDir != "" {
		userCLIDir := filepath.Join(userConfigDir, "cli")
		if err := s.cliRegistry.LoadFromDirectory(userCLIDir); err != nil {
			s.logger.Debug("failed to load user CLI tools", "dir", userCLIDir, "error", err)
		}
	}

	// Project-level CLI tools: .slop-mcp/cli/
	cwd, err := os.Getwd()
	if err == nil {
		projectCLIDir := filepath.Join(cwd, ".slop-mcp", "cli")
		if err := s.cliRegistry.LoadFromDirectory(projectCLIDir); err != nil {
			s.logger.Debug("failed to load project CLI tools", "dir", projectCLIDir, "error", err)
		}
	}

	// Log loaded tools count
	if count := s.cliRegistry.Count(); count > 0 {
		s.logger.Info("loaded CLI tools", "count", count)
	}
}

// RunStdio runs the server using stdio transport.
func (s *Server) RunStdio(ctx context.Context) error {
	if err := s.Start(ctx); err != nil {
		return err
	}

	transport := &mcp.StdioTransport{}
	return s.mcpServer.Run(ctx, transport)
}

// requireBearerToken wraps h so every request must carry
// "Authorization: Bearer <token>". The comparison is constant-time.
func requireBearerToken(token string, h http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if len(got) != len(want) || subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// RunHTTP runs the server using HTTP/SSE transport.
func (s *Server) RunHTTP(ctx context.Context, port int) error {
	if err := s.Start(ctx); err != nil {
		return err
	}

	addr := fmt.Sprintf(":%d", port)

	// Create SSE handler
	sseHandler := mcp.NewSSEHandler(func(r *http.Request) *mcp.Server {
		return s.mcpServer
	}, nil)

	mux := http.NewServeMux()
	mux.Handle("/", sseHandler)
	mux.HandleFunc("/status", s.handleHTTPStatus)
	mux.HandleFunc("/metadata", s.handleHTTPMetadata)

	// Optional bearer-token gate over every route (SSE transport + the
	// diagnostic endpoints). HTTP mode is otherwise unauthenticated and, bound
	// to all interfaces, would expose the MCP inventory and tool descriptions to
	// anything that can reach the port.
	var handler http.Handler = mux
	if token := strings.TrimSpace(os.Getenv("SLOP_MCP_HTTP_TOKEN")); token != "" {
		handler = requireBearerToken(token, mux)
		s.logger.Info("HTTP mode requires bearer token (SLOP_MCP_HTTP_TOKEN)")
	} else {
		s.logger.Warn("HTTP mode is UNAUTHENTICATED; set SLOP_MCP_HTTP_TOKEN to require a bearer token, or bind to a trusted interface only")
	}

	// SSE streams are long-lived: ReadTimeout/WriteTimeout would kill active
	// event streams, so only header-read and keep-alive idle are bounded.
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	s.logger.Info("slop-mcp server running", "addr", addr)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http server shutdown: %w", err)
		}
		return ctx.Err()
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (s *Server) handleHTTPStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_, result, err := s.handleManageMCPs(r.Context(), nil, ManageMCPsInput{Action: "status"})
	writeHTTPJSON(w, result, err)
}

func (s *Server) handleHTTPMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	input := GetMetadataInput{
		MCPName:  r.URL.Query().Get("mcp_name"),
		ToolName: r.URL.Query().Get("tool_name"),
		Verbose:  r.URL.Query().Get("verbose") == "true",
	}
	_, result, err := s.handleGetMetadata(r.Context(), nil, input)
	writeHTTPJSON(w, result, err)
}

func writeHTTPJSON(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

// Close closes all MCP connections and the override store.
func (s *Server) Close() error {
	var errs []error
	if s.overrideStore != nil {
		if err := s.overrideStore.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := s.registry.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// SetOverrideStoreForTesting replaces the override store (closing any existing one)
// and re-registers the registry provider. For use in tests only.
func (s *Server) SetOverrideStoreForTesting(store *overrides.Store) {
	if s.overrideStore != nil {
		_ = s.overrideStore.Close()
	}
	s.overrideStore = store
	s.registry.SetOverrideProvider(&storeOverrideProvider{store: store})
}

// Registry returns the underlying registry.
func (s *Server) Registry() *registry.Registry {
	return s.registry
}

// CLIRegistry returns the CLI tool registry.
func (s *Server) CLIRegistry() *cli.Registry {
	return s.cliRegistry
}

// CallTool calls a tool directly (for testing purposes).
func (s *Server) CallTool(ctx context.Context, toolName string, args map[string]any) (any, error) {
	switch toolName {
	case "search_tools":
		input := SearchToolsInput{
			Query:   getStringArg(args, "query"),
			MCPName: getStringArg(args, "mcp_name"),
		}
		_, result, err := s.handleSearchTools(ctx, nil, input)
		return result, err

	case "execute_tool":
		input := ExecuteToolInput{
			MCPName:    getStringArg(args, "mcp_name"),
			ToolName:   getStringArg(args, "tool_name"),
			Parameters: getMapArg(args, "parameters"),
		}
		_, result, err := s.handleExecuteTool(ctx, nil, input)
		return result, err

	case "run_slop":
		input := RunSlopInput{
			Script:   getStringArg(args, "script"),
			FilePath: getStringArg(args, "file_path"),
			Recipe:   getStringArg(args, "recipe"),
		}
		_, result, err := s.handleRunSlop(ctx, nil, input)
		return result, err

	case "manage_mcps":
		input := ManageMCPsInput{
			Action:  getStringArg(args, "action"),
			Name:    getStringArg(args, "name"),
			Type:    getStringArg(args, "type"),
			Command: getStringArg(args, "command"),
			Args:    getStringSliceArg(args, "args"),
			Env:     getStringMapArg(args, "env"),
			URL:     getStringArg(args, "url"),
			Headers: getStringMapArg(args, "headers"),
			Scope:   getStringArg(args, "scope"),
			Dynamic: getBoolArg(args, "dynamic"),
		}
		_, result, err := s.handleManageMCPs(ctx, nil, input)
		return result, err

	case "get_metadata":
		input := GetMetadataInput{
			MCPName:  getStringArg(args, "mcp_name"),
			ToolName: getStringArg(args, "tool_name"),
			FilePath: getStringArg(args, "file_path"),
			Verbose:  getBoolArg(args, "verbose"),
		}
		_, result, err := s.handleGetMetadata(ctx, nil, input)
		return result, err

	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func getStringArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getMapArg(args map[string]any, key string) map[string]any {
	if v, ok := args[key]; ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}
	return nil
}

func getBoolArg(args map[string]any, key string) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func getStringSliceArg(args map[string]any, key string) []string {
	if v, ok := args[key]; ok {
		switch s := v.(type) {
		case []string:
			return s
		case []any:
			result := make([]string, len(s))
			for i, item := range s {
				if str, ok := item.(string); ok {
					result[i] = str
				}
			}
			return result
		}
	}
	return nil
}

func getStringMapArg(args map[string]any, key string) map[string]string {
	if v, ok := args[key]; ok {
		if m, ok := v.(map[string]string); ok {
			return m
		}
		if m, ok := v.(map[string]any); ok {
			result := make(map[string]string)
			for k, val := range m {
				if s, ok := val.(string); ok {
					result[k] = s
				}
			}
			return result
		}
	}
	return nil
}
