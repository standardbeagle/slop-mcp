package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/anthropics/slop-mcp/internal/config"
	"github.com/anthropics/slop-mcp/internal/registry"
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
func New(cfg *config.Config) (*Server, error) {
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

// Start connects to all configured MCPs and prepares the server.
func (s *Server) Start(ctx context.Context) error {
	// Connect to all configured MCPs
	if err := s.registry.ConnectFromConfig(ctx, s.config); err != nil {
		return fmt.Errorf("failed to connect to MCPs: %w", err)
	}
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
