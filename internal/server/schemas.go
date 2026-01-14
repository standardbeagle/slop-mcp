package server

import "encoding/json"

// Tool schemas manually crafted to pass Claude Code's strict MCP validation.
// The Go SDK's auto-generated schemas use patterns like "type": ["null", "object"]
// which are valid JSON Schema but rejected by Claude Code's validator.

// searchToolsInputSchema is the input schema for search_tools.
var searchToolsInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"query": {
			"type": "string",
			"description": "Search query for tool names and descriptions"
		},
		"mcp_name": {
			"type": "string",
			"description": "Filter to a specific MCP server"
		}
	},
	"additionalProperties": false
}`)

// searchToolsOutputSchema is the output schema for search_tools.
var searchToolsOutputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"tools": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"description": {"type": "string"},
					"mcp_name": {"type": "string"},
					"input_schema": {"type": "object", "additionalProperties": true}
				},
				"required": ["name", "description", "mcp_name"]
			}
		},
		"total": {"type": "integer"}
	},
	"required": ["tools", "total"],
	"additionalProperties": false
}`)

// executeToolInputSchema is the input schema for execute_tool.
var executeToolInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"mcp_name": {
			"type": "string",
			"description": "Target MCP server name"
		},
		"tool_name": {
			"type": "string",
			"description": "Tool to execute on the MCP server"
		},
		"parameters": {
			"type": "object",
			"description": "Tool parameters to pass through",
			"additionalProperties": true
		}
	},
	"required": ["mcp_name", "tool_name"],
	"additionalProperties": false
}`)

// executeToolOutputSchema - execute_tool returns pass-through results, so we use a flexible schema.
var executeToolOutputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": true
}`)

// runSlopInputSchema is the input schema for run_slop.
var runSlopInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"script": {
			"type": "string",
			"description": "Inline SLOP script to execute"
		},
		"file_path": {
			"type": "string",
			"description": "Path to a .slop file to execute"
		}
	},
	"additionalProperties": false
}`)

// runSlopOutputSchema is the output schema for run_slop.
// Using "additionalProperties": true for dynamic result types instead of "true" literal.
var runSlopOutputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"result": {
			"type": "object",
			"additionalProperties": true
		},
		"emitted": {
			"type": "array",
			"items": {
				"type": "object",
				"additionalProperties": true
			}
		}
	},
	"additionalProperties": false
}`)

// manageMCPsInputSchema is the input schema for manage_mcps.
var manageMCPsInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"action": {
			"type": "string",
			"description": "Action to perform: register, unregister, list, or status"
		},
		"name": {
			"type": "string",
			"description": "MCP server name (required for register/unregister)"
		},
		"type": {
			"type": "string",
			"description": "Transport type: command (default), sse, or streamable"
		},
		"command": {
			"type": "string",
			"description": "Command executable for command transport"
		},
		"args": {
			"type": "array",
			"description": "Command arguments",
			"items": {"type": "string"}
		},
		"env": {
			"type": "object",
			"description": "Environment variables",
			"additionalProperties": {"type": "string"}
		},
		"url": {
			"type": "string",
			"description": "Server URL for HTTP transports"
		},
		"headers": {
			"type": "object",
			"description": "HTTP headers for HTTP transports",
			"additionalProperties": {"type": "string"}
		},
		"scope": {
			"type": "string",
			"description": "Where to save: memory (default, runtime only), user (~/.config/slop-mcp/config.kdl), or project (.slop-mcp.kdl)"
		}
	},
	"required": ["action"],
	"additionalProperties": false
}`)

// manageMCPsOutputSchema is the output schema for manage_mcps.
var manageMCPsOutputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"message": {"type": "string"},
		"mcps": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"type": {"type": "string"},
					"connected": {"type": "boolean"},
					"tool_count": {"type": "integer"},
					"source": {"type": "string"}
				},
				"required": ["name", "type", "connected", "tool_count", "source"]
			}
		},
		"status": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"type": {"type": "string"},
					"state": {"type": "string"},
					"source": {"type": "string"},
					"tool_count": {"type": "integer"},
					"error": {"type": "string"}
				},
				"required": ["name", "type", "state", "source"]
			}
		}
	},
	"additionalProperties": false
}`)

// authMCPInputSchema is the input schema for auth_mcp.
var authMCPInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"action": {
			"type": "string",
			"description": "Action to perform: login, logout, status, or list"
		},
		"name": {
			"type": "string",
			"description": "MCP server name (required for login/logout/status)"
		}
	},
	"required": ["action"],
	"additionalProperties": false
}`)

// authMCPOutputSchema is the output schema for auth_mcp.
// Note: "status" is now a plain object (not nullable) - it will be omitted if not present.
var authMCPOutputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"message": {"type": "string"},
		"status": {
			"type": "object",
			"properties": {
				"server_name": {"type": "string"},
				"server_url": {"type": "string"},
				"is_authenticated": {"type": "boolean"},
				"expires_at": {"type": "string"},
				"is_expired": {"type": "boolean"},
				"has_refresh_token": {"type": "boolean"}
			},
			"required": ["server_name", "is_authenticated"]
		},
		"tokens": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"server_name": {"type": "string"},
					"server_url": {"type": "string"},
					"is_authenticated": {"type": "boolean"},
					"expires_at": {"type": "string"},
					"is_expired": {"type": "boolean"},
					"has_refresh_token": {"type": "boolean"}
				},
				"required": ["server_name", "is_authenticated"]
			}
		}
	},
	"additionalProperties": false
}`)

// getMetadataInputSchema is the input schema for get_metadata.
var getMetadataInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"mcp_name": {
			"type": "string",
			"description": "Filter to a specific MCP server (optional)"
		},
		"tool_name": {
			"type": "string",
			"description": "Filter to a specific tool by name (optional)"
		},
		"file_path": {
			"type": "string",
			"description": "Path to write metadata to (optional)"
		},
		"verbose": {
			"type": "boolean",
			"description": "Include full input schemas for all tools (default: false, schemas only included when querying specific mcp_name + tool_name)"
		}
	},
	"additionalProperties": false
}`)

// getMetadataOutputSchema is the output schema for get_metadata.
var getMetadataOutputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"metadata": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"type": {"type": "string"},
					"state": {"type": "string"},
					"source": {"type": "string"},
					"error": {"type": "string"},
					"tools": {
						"type": "array",
						"items": {
							"type": "object",
							"properties": {
								"name": {"type": "string"},
								"description": {"type": "string"},
								"mcp_name": {"type": "string"},
								"input_schema": {"type": "object", "additionalProperties": true}
							},
							"required": ["name", "description", "mcp_name"]
						}
					},
					"prompts": {
						"type": "array",
						"items": {
							"type": "object",
							"properties": {
								"name": {"type": "string"},
								"description": {"type": "string"},
								"arguments": {
									"type": "array",
									"items": {
										"type": "object",
										"properties": {
											"name": {"type": "string"},
											"description": {"type": "string"},
											"required": {"type": "boolean"}
										},
										"required": ["name"]
									}
								}
							},
							"required": ["name"]
						}
					},
					"resources": {
						"type": "array",
						"items": {
							"type": "object",
							"properties": {
								"uri": {"type": "string"},
								"name": {"type": "string"},
								"description": {"type": "string"},
								"mime_type": {"type": "string"}
							},
							"required": ["uri", "name"]
						}
					},
					"resource_templates": {
						"type": "array",
						"items": {
							"type": "object",
							"properties": {
								"uri_template": {"type": "string"},
								"name": {"type": "string"},
								"description": {"type": "string"},
								"mime_type": {"type": "string"}
							},
							"required": ["uri_template", "name"]
						}
					}
				},
				"required": ["name", "type", "state", "source"]
			}
		},
		"total": {"type": "integer"},
		"file_path": {"type": "string"}
	},
	"required": ["metadata", "total"],
	"additionalProperties": false
}`)
