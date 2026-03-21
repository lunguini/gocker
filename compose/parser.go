package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads and parses a compose file. If file is empty, it searches for
// compose.yaml, compose.yml, docker-compose.yml, or docker-compose.yaml.
func Load(file string) (*ComposeFile, string, error) {
	if file == "" {
		candidates := []string{
			"compose.yaml",
			"compose.yml",
			"docker-compose.yml",
			"docker-compose.yaml",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				file = c
				break
			}
		}
		if file == "" {
			return nil, "", fmt.Errorf("no compose file found (tried compose.yaml, compose.yml, docker-compose.yml, docker-compose.yaml)")
		}
	}

	absFile, err := filepath.Abs(file)
	if err != nil {
		return nil, "", err
	}

	data, err := os.ReadFile(absFile)
	if err != nil {
		return nil, "", fmt.Errorf("reading %s: %w", file, err)
	}

	// Substitute environment variables: ${VAR} and $VAR
	expanded := os.Expand(string(data), func(key string) string {
		// Handle ${VAR:-default} syntax
		if idx := strings.Index(key, ":-"); idx != -1 {
			envKey := key[:idx]
			defaultVal := key[idx+2:]
			if val, ok := os.LookupEnv(envKey); ok {
				return val
			}
			return defaultVal
		}
		// Handle ${VAR-default} syntax (only if unset, not if empty)
		if idx := strings.Index(key, "-"); idx != -1 {
			envKey := key[:idx]
			defaultVal := key[idx+1:]
			if val, ok := os.LookupEnv(envKey); ok {
				return val
			}
			return defaultVal
		}
		return os.Getenv(key)
	})

	var cf ComposeFile
	if err := yaml.Unmarshal([]byte(expanded), &cf); err != nil {
		return nil, "", fmt.Errorf("parsing %s: %w", file, err)
	}

	if len(cf.Services) == 0 {
		return nil, "", fmt.Errorf("%s: no services defined", file)
	}

	// Load env_file entries for each service
	dir := filepath.Dir(absFile)
	for name, svc := range cf.Services {
		for _, envFile := range svc.EnvFile {
			envPath := envFile
			if !filepath.IsAbs(envPath) {
				envPath = filepath.Join(dir, envPath)
			}
			envVars, err := readEnvFile(envPath)
			if err != nil {
				return nil, "", fmt.Errorf("service %q: %w", name, err)
			}
			if svc.Environment == nil {
				svc.Environment = make(Environment)
			}
			for k, v := range envVars {
				// Explicit environment values take precedence over env_file
				if _, exists := svc.Environment[k]; !exists {
					svc.Environment[k] = v
				}
			}
			cf.Services[name] = svc
		}
	}

	return &cf, absFile, nil
}

func readEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading env file %s: %w", path, err)
	}
	env := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		env[k] = v
	}
	return env, nil
}

// DependencyOrder returns service names in startup order (dependencies first).
// Uses Kahn's algorithm for topological sort.
func DependencyOrder(cf *ComposeFile) ([]string, error) {
	// Build adjacency list and in-degree count
	inDegree := make(map[string]int)
	dependents := make(map[string][]string) // dep -> services that depend on it

	for name := range cf.Services {
		inDegree[name] = 0
	}

	for name, svc := range cf.Services {
		for _, dep := range svc.DependsOn {
			if _, ok := cf.Services[dep]; !ok {
				return nil, fmt.Errorf("service %q depends on %q which is not defined", name, dep)
			}
			inDegree[name]++
			dependents[dep] = append(dependents[dep], name)
		}
	}

	// Kahn's algorithm
	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}

	var order []string
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		order = append(order, curr)

		for _, dep := range dependents[curr] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(cf.Services) {
		return nil, fmt.Errorf("circular dependency detected among services")
	}

	return order, nil
}

// ProjectName derives the project name from the compose file's directory.
func ProjectName(composeFile string) string {
	dir := filepath.Dir(composeFile)
	return sanitizeName(filepath.Base(dir))
}

func sanitizeName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if result == "" {
		return "default"
	}
	return result
}
