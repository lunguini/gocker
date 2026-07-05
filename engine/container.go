package engine

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/lunguini/gocker/internal/jsonx"
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

// ContainerCreate runs `container create <args>` and returns the new
// container's ID (Apple's CLI prints it on stdout). Unlike ContainerRun this
// does not start the container — the API create/start split relies on it.
func (e *Engine) ContainerCreate(ctx context.Context, args []string) (string, error) {
	cmdArgs := append([]string{"create"}, args...)
	stdout, stderr, err := e.Exec(ctx, cmdArgs...)
	if err != nil {
		return "", cliError(stderr, err)
	}
	return strings.TrimSpace(string(stdout)), nil
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
		for line := range strings.SplitSeq(trimmed, "\n") {
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
		ID:     jsonx.GetString(config, "id"),
		Name:   jsonx.GetString(config, "id"),
		Status: jsonx.GetString(c, "status"),
		State:  jsonx.GetString(c, "status"),
	}

	// Image reference: configuration.image.reference
	if imgMap, ok := config["image"].(map[string]any); ok {
		info.Image = jsonx.GetString(imgMap, "reference")
	}

	// Command: configuration.initProcess.executable
	if initProc, ok := config["initProcess"].(map[string]any); ok {
		info.Command = jsonx.GetString(initProc, "executable")
	}

	// IP: first network's ipv4Address
	if networks, ok := c["networks"].([]any); ok && len(networks) > 0 {
		if net, ok := networks[0].(map[string]any); ok {
			ip := jsonx.GetString(net, "ipv4Address")
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
