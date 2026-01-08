package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/standardbeagle/slop-mcp/internal/config"
	"github.com/standardbeagle/slop-mcp/internal/registry"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "slop-mcp"
	serverVersion = "0.1.0"
)

// Server is the slop-mcp server.
type Server struct {
	mcpServer *mcp.Server
	registry  *registry.Registry
	config    *config.Config
}

// New creates a new Server with the given config.
func New(ctx context.Context, mcps []config.MCPConfig) (*Server, error) {
	s := &Server{
		registry: registry.New(),
		config:   config.NewConfig(),
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
		registry: registry.New(),
		config:   cfg,
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
func (s *Server) Start(ctx context.Context) error {
	// Register all configured MCPs first (so status shows them immediately)
	for _, cfg := range s.config.MCPs {
		s.registry.SetConfigured(cfg)
	}

	// Connect to MCPs in background to avoid blocking server startup
	go func() {
		if err := s.registry.ConnectFromConfig(ctx, s.config); err != nil {
			fmt.Printf("Warning: error connecting to MCPs: %v\n", err)
		}
	}()
	return nil
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

	fmt.Printf("slop-mcp server running on http://localhost%s\n", addr)
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
