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

// Load loads and merges user and project configs.
func Load(projectDir string) (*Config, error) {
	user, err := LoadUserConfig()
	if err != nil {
		return nil, err
	}

	project, err := LoadProjectConfig(projectDir)
	if err != nil {
		return nil, err
	}

	return Merge(user, project), nil
}
