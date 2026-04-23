package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/lunguini/gocker/engine"
)

// resolveImageReferences looks up the user's input in the image list and
// returns the full references that should be passed to the underlying runtime
// for deletion.
//
// Resolution rules, in order:
//  1. Input contains ":" (explicit tag) or "@" (digest) → treated as an exact
//     reference. If it matches an image exactly, return it; otherwise pass it
//     through unchanged so the runtime's own error message flows back.
//  2. Input looks like an image ID (>= 6 hex chars) and uniquely prefixes one
//     image's ID → return that image's full reference.
//  3. Input matches one or more images by repository name (no tag) → return
//     every matching "<name>:<tag>" reference so all tags get removed.
//  4. No match → pass input through; the runtime will report "no such image".
func resolveImageReferences(ctx context.Context, eng engine.Runtime, input string) ([]string, error) {
	if input == "" {
		return nil, fmt.Errorf("empty image reference")
	}

	images, err := eng.ImageList(ctx)
	if err != nil {
		return nil, err
	}

	fullRef := func(img engine.ImageInfo) string {
		if img.Tag != "" {
			return img.Name + ":" + img.Tag
		}
		return img.Name
	}

	// Rule 1: image ID prefix. Check this first so "sha256:<hex>" and bare
	// hex strings resolve by ID rather than being treated as a tag.
	normInput := stripHashAlgo(input)
	if looksLikeImageID(normInput) {
		var idMatches []string
		for _, img := range images {
			if img.ID == "" {
				continue
			}
			if strings.HasPrefix(stripHashAlgo(img.ID), normInput) {
				idMatches = append(idMatches, fullRef(img))
			}
		}
		if len(idMatches) == 1 {
			return idMatches, nil
		}
		if len(idMatches) > 1 {
			return nil, fmt.Errorf("ambiguous image ID %q (matches %d images); use a longer prefix", input, len(idMatches))
		}
		// fall through — maybe it's actually a repo name that happens to look hex
	}

	// Rule 2: explicit tag (name:tag) or digest (name@sha256:...) → exact ref match.
	if strings.ContainsAny(input, ":@") {
		for _, img := range images {
			if fullRef(img) == input || img.Name+"@"+img.Digest == input {
				return []string{fullRef(img)}, nil
			}
		}
		return []string{input}, nil
	}

	// Rule 3: repository name (no tag). Match all tags.
	var nameMatches []string
	for _, img := range images {
		if img.Name == input {
			nameMatches = append(nameMatches, fullRef(img))
		}
	}
	if len(nameMatches) > 0 {
		return nameMatches, nil
	}

	// Rule 4: pass through unchanged; runtime will report the error.
	return []string{input}, nil
}

// removeImages resolves each input to one or more full image references and
// calls the backend's ImageRemove for each, printing a "Deleted: <ref>" line
// per successful removal. Returns the first error encountered.
func removeImages(ctx context.Context, eng engine.Runtime, inputs []string) error {
	for _, input := range inputs {
		refs, err := resolveImageReferences(ctx, eng, input)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			if err := eng.ImageRemove(ctx, ref); err != nil {
				return fmt.Errorf("removing %s: %w", ref, err)
			}
			fmt.Println("Deleted:", ref)
		}
	}
	return nil
}

// stripHashAlgo returns s without a "sha256:" / "sha512:" / etc. prefix if
// present; otherwise s unchanged.
func stripHashAlgo(s string) string {
	if i := strings.Index(s, ":"); i > 0 && i < len(s)-1 {
		prefix := s[:i]
		// Only strip when the prefix looks like a hash algo name, not a tag
		// (tags can contain letters/digits/dots/underscores/hyphens).
		if prefix == "sha256" || prefix == "sha512" || prefix == "sha1" {
			return s[i+1:]
		}
	}
	return s
}

// looksLikeImageID returns true if s is 6-64 hex characters — the shape of a
// (possibly-truncated) image digest. Repository names can't be pure hex of
// that length in practice, so this is a safe heuristic.
func looksLikeImageID(s string) bool {
	if len(s) < 6 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		isDigit := r >= '0' && r <= '9'
		isLowerHex := r >= 'a' && r <= 'f'
		if !isDigit && !isLowerHex {
			return false
		}
	}
	return true
}
