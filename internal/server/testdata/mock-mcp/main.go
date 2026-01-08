// Mock MCP server for integration testing
// Returns deterministic responses without npx/network dependencies
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// JSON-RPC types
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP types
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

type Capabilities struct {
	Tools map[string]any `json:"tools,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"inputSchema"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type CallToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type CallToolResult struct {
	Content []Content `json:"content"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer size for large messages
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, len(buf))

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}

		resp := handleRequest(req)
		if resp != nil {
			respBytes, _ := json.Marshal(resp)
			os.Stdout.WriteString(string(respBytes) + "\n")
			os.Stdout.Sync()
		}
	}
}

func handleRequest(req Request) *Response {
	switch req.Method {
	case "initialize":
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: InitializeResult{
				ProtocolVersion: "2024-11-05",
				Capabilities: Capabilities{
					Tools: map[string]any{},
				},
				ServerInfo: ServerInfo{
					Name:    "mock-everything",
					Version: "1.0.0",
				},
			},
		}

	case "notifications/initialized":
		// No response needed for notification
		return nil

	case "tools/list":
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: ToolsListResult{
				Tools: []Tool{
					{
						Name:        "echo",
						Description: "Echoes back the input message",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"message": map[string]any{
									"type":        "string",
									"description": "Message to echo",
								},
							},
							"required": []string{"message"},
						},
					},
					{
						Name:        "add",
						Description: "Adds two numbers",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"a": map[string]any{
									"type":        "number",
									"description": "First number",
								},
								"b": map[string]any{
									"type":        "number",
									"description": "Second number",
								},
							},
							"required": []string{"a", "b"},
						},
					},
				},
			},
		}

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &Error{
					Code:    -32602,
					Message: "Invalid params",
				},
			}
		}

		return handleToolCall(req.ID, params)

	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &Error{
				Code:    -32601,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

func handleToolCall(id any, params CallToolParams) *Response {
	switch params.Name {
	case "echo":
		message := ""
		if msg, ok := params.Arguments["message"].(string); ok {
			message = msg
		}
		return &Response{
			JSONRPC: "2.0",
			ID:      id,
			Result: CallToolResult{
				Content: []Content{
					{Type: "text", Text: fmt.Sprintf("Echo: %s", message)},
				},
			},
		}

	case "add":
		a := getNumber(params.Arguments["a"])
		b := getNumber(params.Arguments["b"])
		sum := a + b
		return &Response{
			JSONRPC: "2.0",
			ID:      id,
			Result: CallToolResult{
				Content: []Content{
					{Type: "text", Text: fmt.Sprintf("The sum of %.0f and %.0f is %.0f.", a, b, sum)},
				},
			},
		}

	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      id,
			Error: &Error{
				Code:    -32602,
				Message: fmt.Sprintf("Unknown tool: %s", params.Name),
			},
		}
	}
}

func getNumber(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}
