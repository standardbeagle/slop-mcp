package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/standardbeagle/slop-mcp/internal/cli"
	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/logging"
	"github.com/standardbeagle/slop-mcp/internal/registry"
)

const (
	serverName    = "slop-mcp"
	serverVersion = "0.9.3"
)

// Server is the slop-mcp server.
type Server struct {
	mcpServer   *mcp.Server
	registry    *registry.Registry
	cliRegistry *cli.Registry
	config      *config.Config
	logger      logging.Logger
}

// New creates a new Server with the given config.
func New(ctx context.Context, mcps []config.MCPConfig) (*Server, error) {
	s := &Server{
		registry:    registry.New(),
		cliRegistry: cli.NewRegistry(),
		config:      config.NewConfig(),
		logger:      logging.Default(),
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
	s := &Server{
		registry:    registry.New(),
		cliRegistry: cli.NewRegistry(),
		config:      cfg,
		logger:      logging.Default(),
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

// loadCLITools loads CLI tool definitions from standard directories.
func (s *Server) loadCLITools() {
	// User-level CLI tools: ~/.config/slop-mcp/cli/
	userConfigDir := filepath.Dir(config.UserConfigPath())
	userCLIDir := filepath.Join(userConfigDir, "cli")
	if err := s.cliRegistry.LoadFromDirectory(userCLIDir); err != nil {
		s.logger.Debug("failed to load user CLI tools", "dir", userCLIDir, "error", err)
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

	s.logger.Info("slop-mcp server running", "addr", addr)
	return http.ListenAndServe(addr, mux)
}

// Close closes all MCP connections.
func (s *Server) Close() error {
	return s.registry.Close()
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
			Dynamic: getBoolArg(args, "dynamic"),
		}
		_, result, err := s.handleManageMCPs(ctx, nil, input)
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
