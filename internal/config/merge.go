package config

// Merge combines user and project configs.
// Project config takes precedence over user config for the same MCP name.
// For LLM config: providers are merged by name (later wins), non-zero scalar
// fields override, but chain is only replaced when the later layer specifies
// a non-empty chain. This lets users override providers without re-listing chain.
// For Agents: merged by name, later layer wins per name.
func Merge(user, project *Config) *Config {
	merged := NewConfig()

	// Start with user config
	if user != nil {
		for name, cfg := range user.MCPs {
			merged.MCPs[name] = cfg
		}
		mergeLLM(merged, user.LLM)
		for name, agent := range user.Agents {
			merged.Agents[name] = agent
		}
	}

	// Override with project config
	if project != nil {
		for name, cfg := range project.MCPs {
			merged.MCPs[name] = cfg
		}
		mergeLLM(merged, project.LLM)
		for name, agent := range project.Agents {
			merged.Agents[name] = agent
		}
	}

	return merged
}

// mergeLLM merges an incoming LLM config layer into the destination config.
// Providers are merged by name. Chain is only replaced when the incoming
// layer has a non-empty chain; this lets later layers override providers
// without having to repeat the chain declaration.
func mergeLLM(dst *Config, incoming *LLMConfig) {
	if incoming == nil {
		return
	}
	if dst.LLM == nil {
		// No existing LLM config; copy incoming wholesale.
		llm := *incoming
		providers := make(map[string]*LLMProviderConfig, len(incoming.Providers))
		for k, v := range incoming.Providers {
			p := *v
			providers[k] = &p
		}
		llm.Providers = providers
		dst.LLM = &llm
		return
	}
	// Merge into existing LLM config.
	if len(incoming.Chain) > 0 {
		dst.LLM.Chain = incoming.Chain
	}
	if incoming.ToolTimeoutSec != 0 {
		dst.LLM.ToolTimeoutSec = incoming.ToolTimeoutSec
	}
	// Merge providers by name: later layer wins per provider.
	for name, p := range incoming.Providers {
		p2 := *p
		dst.LLM.Providers[name] = &p2
	}
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
