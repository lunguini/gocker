package engine

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/lunguini/gocker/internal/jsonx"
)

func (e *Engine) NetworkCreate(ctx context.Context, name string, labels map[string]string) error {
	args := []string{"network", "create"}
	args = append(args, labelArgs(labels)...)
	args = append(args, name)
	_, stderr, err := e.Exec(ctx, args...)
	if err != nil {
		return cliError(stderr, err)
	}
	return nil
}

// labelArgs emits --label k=v pairs in a stable order (sorted by key) so the
// resulting argv is deterministic across runs — helps tests and diffs.
func labelArgs(labels map[string]string) []string {
	if len(labels) == 0 {
		return nil
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	args := make([]string, 0, 2*len(keys))
	for _, k := range keys {
		args = append(args, "--label", k+"="+labels[k])
	}
	return args
}

func (e *Engine) NetworkList(ctx context.Context) ([]NetworkInfo, error) {
	stdout, stderr, err := e.Exec(ctx, "network", "list", "--format", "json")
	if err != nil {
		return nil, cliError(stderr, err)
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
		for line := range strings.SplitSeq(trimmed, "\n") {
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
		name := jsonx.GetString(n, "name", "Name")
		id := jsonx.GetString(n, "id", "ID", "Id")
		if name == "" {
			name = id
		}
		result = append(result, NetworkInfo{
			ID:     id,
			Name:   name,
			Driver: jsonx.GetString(n, "driver", "Driver"),
			Scope:  jsonx.GetString(n, "scope", "Scope"),
			Labels: jsonx.ExtractLabelsFromAny(n),
		})
	}
	return result, nil
}

func (e *Engine) NetworkRemove(ctx context.Context, name string) error {
	_, stderr, err := e.Exec(ctx, "network", "delete", name)
	if err != nil {
		return cliError(stderr, err)
	}
	return nil
}

func (e *Engine) NetworkConnect(ctx context.Context, network, container string) error {
	_, stderr, err := e.Exec(ctx, "network", "connect", network, container)
	if err != nil {
		return cliError(stderr, err)
	}
	return nil
}

func (e *Engine) NetworkDisconnect(ctx context.Context, network, container string) error {
	_, stderr, err := e.Exec(ctx, "network", "disconnect", network, container)
	if err != nil {
		return cliError(stderr, err)
	}
	return nil
}

func (e *Engine) NetworkInspect(ctx context.Context, name string) ([]byte, error) {
	stdout, stderr, err := e.Exec(ctx, "network", "inspect", name)
	if err != nil {
		return nil, cliError(stderr, err)
	}
	return stdout, nil
}
