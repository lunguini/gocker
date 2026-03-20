package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (e *Engine) ContainerRun(ctx context.Context, args []string, interactive bool) error {
	cmdArgs := append([]string{"run"}, args...)
	if interactive {
		return e.ExecInteractive(ctx, cmdArgs...)
	}
	stdout, stderr, err := e.Exec(ctx, cmdArgs...)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	out := strings.TrimSpace(string(stdout))
	if out != "" {
		fmt.Println(out)
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
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
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
		info := ContainerInfo{
			ID:      getString(c, "id", "ID", "Id"),
			Name:    getString(c, "name", "Name"),
			Image:   getString(c, "image", "Image"),
			State:   getString(c, "state", "State", "status", "Status"),
			Status:  getString(c, "status", "Status", "state", "State"),
			IP:      getString(c, "ip", "IP", "ipAddress", "IPAddress"),
			Ports:   getString(c, "ports", "Ports"),
			Command: getString(c, "command", "Command", "cmd", "Cmd"),
		}
		if created := getString(c, "created", "Created", "createdAt", "CreatedAt"); created != "" {
			if t, err := time.Parse(time.RFC3339, created); err == nil {
				info.Created = t
			}
		}
		result = append(result, info)
	}
	return result, nil
}

func getString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

func (e *Engine) ContainerStop(ctx context.Context, nameOrID string) error {
	_, stderr, err := e.Exec(ctx, "stop", nameOrID)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

func (e *Engine) ContainerStart(ctx context.Context, nameOrID string) error {
	_, stderr, err := e.Exec(ctx, "start", nameOrID)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

func (e *Engine) ContainerRemove(ctx context.Context, nameOrID string, force bool) error {
	if force {
		_ = e.ContainerStop(ctx, nameOrID)
	}
	_, stderr, err := e.Exec(ctx, "delete", nameOrID)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

func (e *Engine) ContainerExec(ctx context.Context, nameOrID string, args []string, interactive bool) error {
	cmdArgs := append([]string{"exec", nameOrID}, args...)
	if interactive {
		return e.ExecInteractive(ctx, cmdArgs...)
	}
	stdout, stderr, err := e.Exec(ctx, cmdArgs...)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	out := strings.TrimSpace(string(stdout))
	if out != "" {
		fmt.Println(out)
	}
	return nil
}

func (e *Engine) ContainerLogs(ctx context.Context, nameOrID string, follow bool) error {
	args := []string{"logs", nameOrID}
	if follow {
		args = append(args, "--follow")
		return e.ExecInteractive(ctx, args...)
	}
	stdout, stderr, err := e.Exec(ctx, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	out := string(stdout) + string(stderr)
	if out != "" {
		fmt.Print(out)
	}
	return nil
}

func (e *Engine) ContainerInspect(ctx context.Context, nameOrID string) ([]byte, error) {
	stdout, stderr, err := e.Exec(ctx, "inspect", nameOrID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return stdout, nil
}
