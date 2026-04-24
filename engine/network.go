package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (e *Engine) NetworkCreate(ctx context.Context, name string) error {
	_, stderr, err := e.Exec(ctx, "network", "create", name)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

func (e *Engine) NetworkList(ctx context.Context) ([]NetworkInfo, error) {
	stdout, stderr, err := e.Exec(ctx, "network", "list", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return parseNetworkListJSON(stdout)
}

func parseNetworkListJSON(data []byte) ([]NetworkInfo, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	var networks []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &networks); err != nil {
		networks = nil
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal([]byte(line), &obj); err != nil {
				continue
			}
			networks = append(networks, obj)
		}
	}

	var result []NetworkInfo
	for _, n := range networks {
		// Apple Container CLI uses "id" as the human-readable identifier
		// and doesn't expose a separate "name" field — id IS the name.
		// Fall back to id when name is absent so callers have something
		// to address the network by.
		name := getString(n, "name", "Name")
		id := getString(n, "id", "ID", "Id")
		if name == "" {
			name = id
		}
		result = append(result, NetworkInfo{
			ID:     id,
			Name:   name,
			Driver: getString(n, "driver", "Driver"),
			Scope:  getString(n, "scope", "Scope"),
			Labels: extractLabelsFromAny(n),
		})
	}
	return result, nil
}

// extractLabelsFromAny pulls a labels map out of a raw JSON object, checking
// the common top-level keys and Apple Container CLI's nested config.labels
// location. Returns a non-nil map so JSON marshal emits `{}` instead of
// `null` — Docker SDK clients sometimes choke on null labels.
func extractLabelsFromAny(m map[string]any) map[string]string {
	check := func(mp map[string]any, keys ...string) map[string]string {
		for _, k := range keys {
			raw, ok := mp[k]
			if !ok {
				continue
			}
			if lm, ok := raw.(map[string]any); ok && len(lm) > 0 {
				out := make(map[string]string, len(lm))
				for k2, v := range lm {
					if s, ok := v.(string); ok {
						out[k2] = s
					}
				}
				return out
			}
		}
		return nil
	}
	if out := check(m, "labels", "Labels"); out != nil {
		return out
	}
	for _, nestedKey := range []string{"config", "Config"} {
		if nested, ok := m[nestedKey].(map[string]any); ok {
			if out := check(nested, "labels", "Labels"); out != nil {
				return out
			}
		}
	}
	return map[string]string{}
}

func (e *Engine) NetworkRemove(ctx context.Context, name string) error {
	_, stderr, err := e.Exec(ctx, "network", "delete", name)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

func (e *Engine) NetworkConnect(ctx context.Context, network, container string) error {
	_, stderr, err := e.Exec(ctx, "network", "connect", network, container)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

func (e *Engine) NetworkDisconnect(ctx context.Context, network, container string) error {
	_, stderr, err := e.Exec(ctx, "network", "disconnect", network, container)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

func (e *Engine) NetworkInspect(ctx context.Context, name string) ([]byte, error) {
	stdout, stderr, err := e.Exec(ctx, "network", "inspect", name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return stdout, nil
}
