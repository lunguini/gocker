package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func (e *Engine) ContainerRun(ctx context.Context, args []string, interactive bool) error {
	cmdArgs := append([]string{"run"}, args...)
	if interactive {
		return e.ExecInteractive(ctx, cmdArgs...)
	}
	stderr, err := execPassthrough(ctx, e.Binary, cmdArgs...)
	if err != nil {
		return cliError(stderr, err)
	}
	return nil
}

func (e *Engine) ContainerList(ctx context.Context, all bool) ([]ContainerInfo, error) {
	args := []string{"list", "--format", "json"}
	if all {
		args = append(args, "-a")
	}
	stdout, stderr, err := e.Exec(ctx, args...)
	if err != nil {
		return nil, cliError(stderr, err)
	}
	return parseContainerListJSON(stdout)
}

func parseContainerListJSON(data []byte) ([]ContainerInfo, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	// Try parsing as array first
	var containers []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &containers); err != nil {
		// Try line-by-line JSON objects
		containers = nil
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal([]byte(line), &obj); err != nil {
				continue
			}
			containers = append(containers, obj)
		}
	}

	var result []ContainerInfo
	for _, c := range containers {
		info := containerInfoFromNested(c)
		result = append(result, info)
	}
	return result, nil
}

// containerInfoFromNested extracts ContainerInfo from Apple's container CLI
// JSON, which nests most fields under "configuration".
func containerInfoFromNested(c map[string]any) ContainerInfo {
	config, _ := c["configuration"].(map[string]any)
	if config == nil {
		config = map[string]any{}
	}

	info := ContainerInfo{
		ID:     getString(config, "id"),
		Name:   getString(config, "id"),
		Status: getString(c, "status"),
		State:  getString(c, "status"),
	}

	// Image reference: configuration.image.reference
	if imgMap, ok := config["image"].(map[string]any); ok {
		info.Image = getString(imgMap, "reference")
	}

	// Command: configuration.initProcess.executable
	if initProc, ok := config["initProcess"].(map[string]any); ok {
		info.Command = getString(initProc, "executable")
	}

	// IP: first network's ipv4Address
	if networks, ok := c["networks"].([]any); ok && len(networks) > 0 {
		if net, ok := networks[0].(map[string]any); ok {
			ip := getString(net, "ipv4Address")
			// Strip CIDR suffix (e.g., "192.168.64.3/24" -> "192.168.64.3")
			if idx := strings.Index(ip, "/"); idx != -1 {
				ip = ip[:idx]
			}
			info.IP = ip
		}
	}

	// Started date: startedDate is a Core Data timestamp (seconds since 2001-01-01)
	if started, ok := c["startedDate"].(float64); ok {
		coreDataEpoch := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
		info.Created = coreDataEpoch.Add(time.Duration(started * float64(time.Second)))
	}

	return info
}

// getString looks up the first of keys present in m with a non-null value
// and renders it as a string. A key present with a JSON null value is
// skipped rather than returned as the literal "<nil>" — the next candidate
// key is checked instead. Numbers are formatted without exponent/decimal
// noise (json.Unmarshal decodes all JSON numbers as float64, so an integral
// ID like 12 would otherwise render as "1.2e+01"-style text via %v).
func getString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		if f, ok := v.(float64); ok {
			if f == math.Trunc(f) && !math.IsInf(f, 0) {
				return strconv.FormatFloat(f, 'f', -1, 64)
			}
			return strconv.FormatFloat(f, 'g', -1, 64)
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func (e *Engine) ContainerStop(ctx context.Context, nameOrID string) error {
	_, stderr, err := e.Exec(ctx, "stop", nameOrID)
	if err != nil {
		return cliError(stderr, err)
	}
	return nil
}

func (e *Engine) ContainerStart(ctx context.Context, nameOrID string) error {
	_, stderr, err := e.Exec(ctx, "start", nameOrID)
	if err != nil {
		return cliError(stderr, err)
	}
	return nil
}

func (e *Engine) ContainerRemove(ctx context.Context, nameOrID string, force bool) error {
	if force {
		_ = e.ContainerStop(ctx, nameOrID)
	}
	_, stderr, err := e.Exec(ctx, "delete", nameOrID)
	if err != nil {
		return cliError(stderr, err)
	}
	return nil
}

func (e *Engine) ContainerExec(ctx context.Context, nameOrID string, args []string, interactive bool) error {
	cmdArgs := append([]string{"exec", nameOrID}, args...)
	if interactive {
		return e.ExecInteractive(ctx, cmdArgs...)
	}
	stderr, err := execPassthrough(ctx, e.Binary, cmdArgs...)
	if err != nil {
		return cliError(stderr, err)
	}
	return nil
}

func (e *Engine) ContainerLogs(ctx context.Context, nameOrID string, opts LogsOptions) error {
	args := []string{"logs"}
	args = append(args, LogsFlags(opts)...)
	args = append(args, nameOrID)
	if opts.Follow {
		return e.ExecInteractive(ctx, args...)
	}
	stderr, err := execPassthrough(ctx, e.Binary, args...)
	if err != nil {
		return cliError(stderr, err)
	}
	return nil
}

func (e *Engine) ContainerInspect(ctx context.Context, nameOrID string) ([]byte, error) {
	stdout, stderr, err := e.Exec(ctx, "inspect", nameOrID)
	if err != nil {
		return nil, cliError(stderr, err)
	}
	return stdout, nil
}
