package config

// Merge combines user and project configs.
// Project config takes precedence over user config for the same MCP name.
func Merge(user, project *Config) *Config {
	merged := NewConfig()

	// Start with user config
	if user != nil {
		for name, cfg := range user.MCPs {
			merged.MCPs[name] = cfg
		}
	}

	// Override with project config
	if project != nil {
		for name, cfg := range project.MCPs {
			merged.MCPs[name] = cfg
		}
	}

	return merged
}

// Load loads and merges the three-tier configs in precedence order:
// local (.slop-mcp.local.kdl) > project (.slop-mcp.kdl) > user config.
func Load(projectDir string) (*Config, error) {
	user, err := LoadUserConfig()
	if err != nil {
		return nil, err
	}

	project, err := LoadProjectConfig(projectDir)
	if err != nil {
		return nil, err
	}

	local, err := LoadLocalConfig(projectDir)
	if err != nil {
		return nil, err
	}

	return Merge(Merge(user, project), local), nil
}
