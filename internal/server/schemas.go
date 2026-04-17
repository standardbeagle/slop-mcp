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
			"description": "Search query matched against tool names and descriptions"
		},
		"mcp_name": {
			"type": "string",
			"description": "Limit results to one MCP"
		},
		"limit": {
			"type": "integer",
			"description": "Max results (default: 20, max: 100)"
		},
		"offset": {
			"type": "integer",
			"description": "Results to skip for pagination (default: 0)"
		}
	},
	"additionalProperties": false
}`)

// executeToolInputSchema is the input schema for execute_tool.
var executeToolInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"mcp_name": {
			"type": "string",
			"description": "Target MCP name"
		},
		"tool_name": {
			"type": "string",
			"description": "Tool to execute"
		},
		"parameters": {
			"type": "object",
			"description": "Parameters passed to tool verbatim",
			"additionalProperties": true
		}
	},
	"required": ["mcp_name", "tool_name"],
	"additionalProperties": false
}`)

// runSlopInputSchema is the input schema for run_slop.
var runSlopInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"script": {
			"type": "string",
			"description": "Inline SLOP script"
		},
		"file_path": {
			"type": "string",
			"description": "Path to .slop file"
		},
		"recipe": {
			"type": "string",
			"description": "Embedded recipe: 'list' to enumerate, or recipe name to load"
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
			"description": "Action: register, unregister, reconnect, list, status, health_check, or list_stale_overrides"
		},
		"name": {
			"type": "string",
			"description": "MCP name (required for register/unregister/reconnect)"
		},
		"type": {
			"type": "string",
			"description": "Transport: command (default), sse, or streamable"
		},
		"command": {
			"type": "string",
			"description": "Executable for command transport"
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
			"description": "Server URL (HTTP transports)"
		},
		"headers": {
			"type": "object",
			"description": "HTTP headers (HTTP transports)",
			"additionalProperties": {"type": "string"}
		},
		"scope": {
			"type": "string",
			"description": "Persistence: memory (default, runtime only), user (~/.config/slop-mcp/config.kdl), project (.slop-mcp.kdl)"
		},
		"dynamic": {
			"type": "boolean",
			"description": "Always re-fetch tool list, skip cache"
		}
	},
	"required": ["action"],
	"additionalProperties": false
}`)

// authMCPInputSchema is the input schema for auth_mcp.
var authMCPInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"action": {
			"type": "string",
			"description": "Action: login, logout, status, or list"
		},
		"name": {
			"type": "string",
			"description": "MCP name (required for login/logout/status)"
		}
	},
	"required": ["action"],
	"additionalProperties": false
}`)

// getMetadataInputSchema is the input schema for get_metadata.
var getMetadataInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"mcp_name": {
			"type": "string",
			"description": "Limit to one MCP"
		},
		"tool_name": {
			"type": "string",
			"description": "Limit to one tool (use with mcp_name)"
		},
		"file_path": {
			"type": "string",
			"description": "Write metadata to file"
		},
		"verbose": {
			"type": "boolean",
			"description": "Include full input schemas (default: false; always included for mcp_name+tool_name queries)"
		}
	},
	"additionalProperties": false
}`)

// slopReferenceInputSchema is the input schema for slop_reference.
var slopReferenceInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"query": {
			"type": "string",
			"description": "Matches name, signature, description"
		},
		"category": {
			"type": "string",
			"description": "Filter: math, string, list, map, random, type, json, regex, time, encoding, functional, crypto, slop"
		},
		"limit": {
			"type": "integer",
			"description": "Max results (default: 10)"
		},
		"verbose": {
			"type": "boolean",
			"description": "Include description, example, returns (default: false, compact shows name+signature)"
		},
		"list_categories": {
			"type": "boolean",
			"description": "Return category counts instead of functions"
		}
	},
	"additionalProperties": false
}`)

// slopHelpInputSchema is the input schema for slop_help.
var slopHelpInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"name": {
			"type": "string",
			"description": "SLOP function name"
		}
	},
	"required": ["name"],
	"additionalProperties": false
}`)

// customizeToolsInputSchema is the input schema for customize_tools.
var customizeToolsInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"action": {
			"type": "string",
			"enum": [
				"set_override",
				"remove_override",
				"list_overrides",
				"define_custom",
				"remove_custom",
				"list_custom",
				"export",
				"import"
			],
			"description": "Action. Per-action args listed in slop-mcp docs."
		},
		"mcp": {
			"type": "string",
			"description": "MCP name (set_override, remove_override, list_overrides, export)"
		},
		"tool": {
			"type": "string",
			"description": "Tool name in MCP (set_override, remove_override)"
		},
		"description": {
			"type": "string",
			"description": "Override description text (set_override, define_custom)"
		},
		"params": {
			"type": "object",
			"additionalProperties": {"type": "string"},
			"description": "Per-param description overrides keyed by property name (set_override)"
		},
		"scope": {
			"type": "string",
			"enum": ["user", "project", "local"],
			"description": "Scope: user, project, local. Default: user for set/define, all for remove."
		},
		"stale_only": {
			"type": "boolean",
			"description": "Only entries whose SourceHash differs from upstream (list_overrides, list_custom)"
		},
		"name": {
			"type": "string",
			"description": "Custom tool name matching ^[a-z][a-z0-9_]{0,63}$ (define_custom, remove_custom)"
		},
		"inputSchema": {
			"type": "object",
			"description": "JSON Schema draft-07 subset for tool arguments (define_custom)"
		},
		"body": {
			"type": "string",
			"description": "SLOP script body (define_custom)"
		},
		"keys": {
			"type": "array",
			"items": {"type": "string"},
			"description": "Glob patterns selecting keys to export"
		},
		"include_custom": {
			"type": "boolean",
			"description": "Include custom tools in export (default true)"
		},
		"data": {
			"type": "string",
			"description": "Import pack as JSON string"
		},
		"overwrite": {
			"type": "boolean",
			"description": "Overwrite existing keys on import (default false)"
		}
	},
	"required": ["action"],
	"additionalProperties": false
}`)

