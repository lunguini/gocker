package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/lunguini/gocker/engine"
)

// pruneReport accumulates the successes and errors from a prune pass so
// callers can print a unified summary at the end. Errors that mean
// "resource is in use" are silently skipped, matching Docker's behavior
// (prune only touches unused things).
type pruneReport struct {
	removed []string
	errors  []string
}

// isInUseError heuristically detects the "resource is currently in use"
// errors returned by Apple Container CLI and nerdctl. These are not real
// prune failures — they mean "this thing is used, skip it."
//
// Apple's `container network delete` wraps the underlying failure in an
// opaque `failed to delete one or more networks: ["<name>"]` message that
// doesn't tell us *why*, so we include it as a soft-skip pattern too.
// Prune is best-effort by design: better to undercount errors than spam
// the user with warnings for networks the backend was right to refuse.
func isInUseError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	patterns := []string{
		"in use",
		"has active endpoints",
		"is being used",
		"has dependent child",
		"referring containers",          // Apple CLI: "cannot delete subnet X with referring containers: Y"
		"cannot delete subnet",          // Apple CLI outer wrapper for the above
		"invalidstate",                  // Apple CLI state-check refusal (e.g. running container holding a volume)
		"failed to delete one or more",  // Apple CLI generic multi-delete wrapper (we see this when a single named resource is in use)
		"delete failed for one or more", // Apple CLI alternate wrapper (newer versions)
	}
	for _, p := range patterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// pruneStoppedContainers removes every container in a non-running terminal
// state (stopped / exited / dead / created).
func pruneStoppedContainers(ctx context.Context, eng engine.Runtime) pruneReport {
	var r pruneReport
	cs, err := eng.ContainerList(ctx, true)
	if err != nil {
		r.errors = append(r.errors, "list containers: "+err.Error())
		return r
	}
	for _, c := range cs {
		state := strings.ToLower(c.State)
		if state == "running" || state == "restarting" || state == "paused" {
			continue
		}
		if err := eng.ContainerRemove(ctx, c.ID, false); err != nil {
			if isInUseError(err) {
				continue
			}
			r.errors = append(r.errors, fmt.Sprintf("remove container %s: %v", displayRef(c.Name, c.ID), err))
			continue
		}
		r.removed = append(r.removed, displayRef(c.Name, c.ID))
	}
	return r
}

// defaultNetworkNames are the built-in networks we never auto-prune.
// Removing these would break every subsequent container create.
var defaultNetworkNames = map[string]struct{}{
	"bridge":  {},
	"host":    {},
	"none":    {},
	"default": {},
}

// pruneUnusedNetworks removes every user-defined network the backend will
// let us remove. "Unused" is enforced by the backend — it refuses to
// remove networks with active endpoints, and we skip those errors silently.
func pruneUnusedNetworks(ctx context.Context, eng engine.Runtime) pruneReport {
	var r pruneReport
	ns, err := eng.NetworkList(ctx)
	if err != nil {
		r.errors = append(r.errors, "list networks: "+err.Error())
		return r
	}
	for _, n := range ns {
		// Skip entries the parser couldn't name — trying to remove "" gives
		// an Apple CLI error with no useful context, and there's nothing
		// the user can do about it anyway.
		ref := n.Name
		if ref == "" {
			ref = n.ID
		}
		if ref == "" {
			continue
		}
		if _, isDefault := defaultNetworkNames[ref]; isDefault {
			continue
		}
		if err := eng.NetworkRemove(ctx, ref); err != nil {
			if isInUseError(err) {
				continue
			}
			r.errors = append(r.errors, fmt.Sprintf("remove network %s: %v", ref, err))
			continue
		}
		r.removed = append(r.removed, ref)
	}
	return r
}

// pruneUnusedVolumes removes every volume the backend will let us remove.
// Same pattern as networks — the backend refuses in-use volumes, so "unused"
// is enforced on its side.
func pruneUnusedVolumes(ctx context.Context, eng engine.Runtime) pruneReport {
	var r pruneReport
	vs, err := eng.VolumeList(ctx)
	if err != nil {
		r.errors = append(r.errors, "list volumes: "+err.Error())
		return r
	}
	for _, v := range vs {
		if v.Name == "" {
			continue
		}
		if err := eng.VolumeRemove(ctx, v.Name); err != nil {
			if isInUseError(err) {
				continue
			}
			r.errors = append(r.errors, fmt.Sprintf("remove volume %s: %v", v.Name, err))
			continue
		}
		r.removed = append(r.removed, v.Name)
	}
	return r
}

// pruneImages removes unused images. If all=false (the default), only
// dangling images are removed (no tag or repo:<none>). If all=true, every
// image the backend will let us delete is removed — the backend refuses
// in-use references, so we skip those errors silently.
func pruneImages(ctx context.Context, eng engine.Runtime, all bool) pruneReport {
	var r pruneReport
	imgs, err := eng.ImageList(ctx)
	if err != nil {
		r.errors = append(r.errors, "list images: "+err.Error())
		return r
	}
	for _, img := range imgs {
		dangling := isDanglingImage(img)
		if !all && !dangling {
			continue
		}
		ref := img.Name
		if img.Tag != "" {
			ref = img.Name + ":" + img.Tag
		}
		if ref == "" || ref == ":" {
			ref = img.ID
		}
		if err := eng.ImageRemove(ctx, ref); err != nil {
			if isInUseError(err) {
				continue
			}
			r.errors = append(r.errors, fmt.Sprintf("remove image %s: %v", ref, err))
			continue
		}
		r.removed = append(r.removed, ref)
	}
	return r
}

// isDanglingImage returns true if the image has no meaningful repo+tag
// reference — it's a stray layer that can't be pulled again by name.
func isDanglingImage(img engine.ImageInfo) bool {
	return img.Name == "" || img.Name == "<none>" || img.Tag == "" || img.Tag == "<none>"
}

// displayRef prefers the name over the id for printing; falls back to the
// short id if the name is empty.
func displayRef(name, id string) string {
	if name != "" {
		return name
	}
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// printPruneReport prints a Docker-style summary line for one resource kind.
// Empty reports print nothing; errors print as warnings.
func printPruneReport(label string, r pruneReport) {
	if len(r.removed) > 0 {
		fmt.Printf("Deleted %s:\n", label)
		for _, n := range r.removed {
			fmt.Printf("  %s\n", n)
		}
	}
	for _, e := range r.errors {
		fmt.Printf("Warning: %s\n", e)
	}
}
