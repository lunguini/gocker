package engine

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
)

func (e *Engine) ImagePull(ctx context.Context, image string, opts ImagePullOpts) error {
	args := buildPullArgs(image, opts, isStdoutTTY())
	// Non-TTY path is almost always the daemon — ExecInteractive's output
	// goes to os.Stdout/Stderr which is /dev/null for the daemon, so errors
	// vanish and the HTTP client just sees "exit status 1". Capture instead.
	if !isStdoutTTY() {
		stdout, stderr, err := e.Exec(ctx, args...)
		if err != nil {
			return wrapRunErr("container image pull", args, stdout, stderr, err)
		}
		return nil
	}
	return e.ExecInteractive(ctx, args...)
}

// buildPullArgs constructs the argv for `container image pull`. Exposed for testing.
func buildPullArgs(image string, opts ImagePullOpts, isTTY bool) []string {
	args := []string{"image", "pull"}
	progress := opts.Progress
	if progress == "" {
		if isTTY {
			progress = "ansi"
		} else {
			progress = "none"
		}
	}
	args = append(args, "--progress", progress)
	if opts.MaxConcurrent > 0 {
		args = append(args, "--max-concurrent-downloads", strconv.Itoa(opts.MaxConcurrent))
	}
	if opts.Platform != "" {
		args = append(args, "--platform", opts.Platform)
	}
	args = append(args, image)
	return args
}

// isStdoutTTY returns true when os.Stdout is a terminal (character device).
func isStdoutTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func (e *Engine) ImagePush(ctx context.Context, image string) error {
	return e.ExecInteractive(ctx, "image", "push", image)
}

func (e *Engine) ImageList(ctx context.Context) ([]ImageInfo, error) {
	stdout, stderr, err := e.Exec(ctx, "image", "list", "--format", "json")
	if err != nil {
		return nil, cliError(stderr, err)
	}
	return parseImageListJSON(stdout)
}

func parseImageListJSON(data []byte) ([]ImageInfo, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	var images []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &images); err != nil {
		images = nil
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal([]byte(line), &obj); err != nil {
				continue
			}
			images = append(images, obj)
		}
	}

	var result []ImageInfo
	for _, img := range images {
		name := getString(img, "repository", "Repository", "name", "Name")
		tag := getString(img, "tag", "Tag")
		digest := getString(img, "digest", "Digest")
		size := getString(img, "size", "Size")
		created := getString(img, "created", "Created", "createdAt", "CreatedAt")

		// Handle Apple container CLI nested format:
		// { "reference": "docker.io/lib/img:tag", "descriptor": { "digest": "sha256:..." }, "fullSize": "28,9 MB" }
		if ref := getString(img, "reference", "Reference"); ref != "" && name == "" {
			name, tag = parseReference(ref)
		}
		if desc, ok := img["descriptor"].(map[string]any); ok {
			if digest == "" {
				digest = getString(desc, "digest", "Digest")
			}
			if annotations, ok := desc["annotations"].(map[string]any); ok {
				if created == "" {
					created = getString(annotations, "org.opencontainers.image.created")
				}
			}
		}
		if size == "" {
			size = getString(img, "fullSize")
		}

		if tag == "" {
			if parts := strings.SplitN(name, ":", 2); len(parts) == 2 {
				name = parts[0]
				tag = parts[1]
			} else {
				tag = "latest"
			}
		}

		info := ImageInfo{
			ID:     getString(img, "id", "ID", "Id"),
			Name:   name,
			Tag:    tag,
			Digest: digest,
			Size:   size,
			Arch:   getString(img, "arch", "Arch", "architecture", "Architecture"),
		}
		if info.ID == "" && digest != "" {
			info.ID = digest
		}
		if created != "" {
			if t, err := time.Parse(time.RFC3339, created); err == nil {
				info.Created = t
			}
		}
		result = append(result, info)
	}
	return result, nil
}

// parseReference splits a full image reference like "docker.io/library/ubuntu:24.04"
// into a name and tag.
func parseReference(ref string) (name, tag string) {
	// Remove docker.io prefix for cleaner display
	name = strings.TrimPrefix(ref, "docker.io/")
	// Remove library/ prefix for official images
	name = strings.TrimPrefix(name, "library/")
	// Split name:tag
	if i := strings.LastIndex(name, ":"); i != -1 {
		tag = name[i+1:]
		name = name[:i]
	}
	return name, tag
}

func (e *Engine) ImageRemove(ctx context.Context, image string) error {
	_, stderr, err := e.Exec(ctx, "image", "delete", image)
	if err != nil {
		return cliError(stderr, err)
	}
	return nil
}

func (e *Engine) ImageBuild(ctx context.Context, args []string) error {
	cmdArgs := append([]string{"build"}, args...)
	return e.ExecInteractive(ctx, cmdArgs...)
}
