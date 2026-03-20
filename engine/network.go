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
		result = append(result, NetworkInfo{
			ID:     getString(n, "id", "ID", "Id"),
			Name:   getString(n, "name", "Name"),
			Driver: getString(n, "driver", "Driver"),
			Scope:  getString(n, "scope", "Scope"),
		})
	}
	return result, nil
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
