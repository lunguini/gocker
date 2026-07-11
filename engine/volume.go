package engine

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/lunguini/gocker/internal/jsonx"
)

func (e *Engine) VolumeCreate(ctx context.Context, name string) error {
	_, stderr, err := e.Exec(ctx, "volume", "create", name)
	if err != nil {
		return cliError(stderr, err)
	}
	return nil
}

func (e *Engine) VolumeList(ctx context.Context) ([]VolumeInfo, error) {
	stdout, stderr, err := e.Exec(ctx, "volume", "list", "--format", "json")
	if err != nil {
		return nil, cliError(stderr, err)
	}
	return parseVolumeListJSON(stdout)
}

func parseVolumeListJSON(data []byte) ([]VolumeInfo, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	var volumes []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &volumes); err != nil {
		volumes = nil
		for line := range strings.SplitSeq(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal([]byte(line), &obj); err != nil {
				continue
			}
			volumes = append(volumes, obj)
		}
	}

	var result []VolumeInfo
	for _, v := range volumes {
		// Apple container CLI 1.1.0+ nests volume fields under
		// "configuration" (same nesting change as container/image list):
		// { "id": "vol", "configuration": { "name": "vol", "driver": "local",
		//   "creationDate": "...", "labels": {...}, "source": "/path/volume.img" } }
		if cfg, ok := v["configuration"].(map[string]any); ok {
			for _, key := range []string{"name", "driver", "labels", "source", "creationDate"} {
				if _, exists := v[key]; !exists {
					if val, ok := cfg[key]; ok {
						v[key] = val
					}
				}
			}
		}
		info := VolumeInfo{
			Name:       jsonx.GetString(v, "name", "Name"),
			Driver:     jsonx.GetString(v, "driver", "Driver"),
			Mountpoint: jsonx.GetString(v, "mountpoint", "Mountpoint", "source"),
			Labels:     jsonx.ExtractLabelsFromAny(v),
		}
		if info.Name == "" {
			info.Name = jsonx.GetString(v, "id", "ID", "Id")
		}
		if created := jsonx.GetString(v, "created", "Created", "createdAt", "CreatedAt", "creationDate"); created != "" {
			if t, err := time.Parse(time.RFC3339, created); err == nil {
				info.Created = t
			}
		}
		result = append(result, info)
	}
	return result, nil
}

func (e *Engine) VolumeRemove(ctx context.Context, name string) error {
	_, stderr, err := e.Exec(ctx, "volume", "delete", name)
	if err != nil {
		return cliError(stderr, err)
	}
	return nil
}

func (e *Engine) VolumeInspect(ctx context.Context, name string) ([]byte, error) {
	stdout, stderr, err := e.Exec(ctx, "volume", "inspect", name)
	if err != nil {
		return nil, cliError(stderr, err)
	}
	return stdout, nil
}
