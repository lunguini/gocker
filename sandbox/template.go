package sandbox

type AgentTemplate struct {
	Name        string
	Image       string
	EntryCmd    []string
	EnvVars     []string // Required env var names (e.g., "ANTHROPIC_API_KEY")
	DefaultArgs []string
}

var builtinTemplates = map[string]*AgentTemplate{
	"claude": {
		Name:        "claude",
		Image:       "docker.io/adyjay/gocker:claude-latest",
		EntryCmd:    []string{"claude", "--dangerously-skip-permissions"},
		EnvVars:     []string{"ANTHROPIC_API_KEY"},
		DefaultArgs: []string{},
	},
	"codex": {
		Name:        "codex",
		Image:       "ubuntu:24.04",
		EntryCmd:    []string{"codex", "--full-auto"},
		EnvVars:     []string{"OPENAI_API_KEY"},
		DefaultArgs: []string{},
	},
}

func GetTemplate(agent string) *AgentTemplate {
	if t, ok := builtinTemplates[agent]; ok {
		return t
	}
	return nil
}
