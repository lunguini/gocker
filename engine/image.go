package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lunguini/gocker/internal/jsonx"
	"github.com/lunguini/gocker/internal/termx"
)

func (e *Engine) ImagePull(ctx context.Context, image string, opts ImagePullOpts) error {
	args := buildPullArgs(image, opts, termx.StdoutIsTTY())
	// Non-TTY path is almost always the daemon — ExecInteractive's output
	// goes to os.Stdout/Stderr which is /dev/null for the daemon, so errors
	// vanish and the HTTP client just sees "exit status 1". Capture instead.
	if !termx.StdoutIsTTY() {
		stdout, stderr, err := e.Exec(ctx, args...)
		if err != nil {
			return wrapRunErr("container image pull", args, stdout, stderr, err)
		}
		return nil
	}
	return e.execInteractiveTee(ctx, args...)
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
		for line := range strings.SplitSeq(trimmed, "\n") {
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
		name := jsonx.GetString(img, "repository", "Repository", "name", "Name")
		tag := jsonx.GetString(img, "tag", "Tag")
		digest := jsonx.GetString(img, "digest", "Digest")
		size := jsonx.GetString(img, "size", "Size")
		created := jsonx.GetString(img, "created", "Created", "createdAt", "CreatedAt")

		// Handle Apple container CLI nested format:
		// { "reference": "docker.io/lib/img:tag", "descriptor": { "digest": "sha256:..." }, "fullSize": "28,9 MB" }
		if ref := jsonx.GetString(img, "reference", "Reference"); ref != "" && name == "" {
			name, tag = parseReference(ref)
		}
		if desc, ok := img["descriptor"].(map[string]any); ok {
			if digest == "" {
				digest = jsonx.GetString(desc, "digest", "Digest")
			}
			if annotations, ok := desc["annotations"].(map[string]any); ok {
				if created == "" {
					created = jsonx.GetString(annotations, "org.opencontainers.image.created")
				}
			}
		}
		if size == "" {
			size = jsonx.GetString(img, "fullSize")
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
			ID:        jsonx.GetString(img, "id", "ID", "Id"),
			Name:      name,
			Tag:       tag,
			Digest:    digest,
			Size:      size,
			SizeBytes: parseSizeString(size),
			Arch:      jsonx.GetString(img, "arch", "Arch", "architecture", "Architecture"),
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

// parseSizeString converts a human-readable image size into bytes. Apple's
// container CLI reports sizes like "28,9 MB" — decimal units with a comma
// decimal separator (likely locale-influenced formatting from Swift/ICU).
// nerdctl reports sizes like "28.9MB" (decimal) or "1.2GiB" (binary), dot
// decimal separator, no space before the unit. Both shapes are handled here;
// unparseable input returns 0 (callers already keep the raw string in
// ImageInfo.Size for display).
func parseSizeString(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	i := 0
	for i < len(s) {
		c := s[i]
		if (c >= '0' && c <= '9') || c == '.' || c == ',' {
			i++
			continue
		}
		break
	}
	numPart := s[:i]
	unit := strings.TrimSpace(s[i:])
	if numPart == "" {
		return 0
	}
	// Apple's comma decimal separator only ever appears once, with no
	// thousands grouping in observed output — safe to normalize to a dot.
	numPart = strings.Replace(numPart, ",", ".", 1)
	value, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0
	}

	const (
		kb = 1000
		mb = kb * 1000
		gb = mb * 1000
		tb = gb * 1000
		ki = 1024
		mi = ki * 1024
		gi = mi * 1024
		ti = gi * 1024
	)
	var multiplier float64
	switch strings.ToLower(unit) {
	case "", "b", "byte", "bytes":
		multiplier = 1
	case "kb":
		multiplier = kb
	case "kib":
		multiplier = ki
	case "mb":
		multiplier = mb
	case "mib":
		multiplier = mi
	case "gb":
		multiplier = gb
	case "gib":
		multiplier = gi
	case "tb":
		multiplier = tb
	case "tib":
		multiplier = ti
	default:
		return 0
	}
	return int64(value * multiplier)
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
		delErr := cliError(stderr, err)
		// Apple's `container image delete` reports a generic "failed to
		// delete one or more images" with no not-found marker, so the text
		// alone can't be classified. `image inspect` does say "Image not
		// found" — use it to disambiguate so the API layer can return 404
		// for missing images (a documented Docker-compat invariant).
		if !errors.Is(delErr, ErrNotFound) {
			if _, istderr, ierr := e.Exec(ctx, "image", "inspect", image); ierr != nil && errors.Is(cliError(istderr, ierr), ErrNotFound) {
				return fmt.Errorf("no such image %q: %w", image, ErrNotFound)
			}
		}
		return delErr
	}
	return nil
}

func (e *Engine) ImageBuild(ctx context.Context, args []string) error {
	cmdArgs := append([]string{"build"}, args...)
	return e.ExecInteractive(ctx, cmdArgs...)
}
