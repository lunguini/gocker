package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/lunguini/gocker/engine"
)

func (s *Server) handleImageList(w http.ResponseWriter, r *http.Request) {
	images, err := s.eng.ImageList(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var result []ImageJSON
	for _, img := range images {
		repoTag := img.Name
		if img.Tag != "" {
			repoTag = img.Name + ":" + img.Tag
		}
		result = append(result, ImageJSON{
			ID:       img.ID,
			RepoTags: []string{repoTag},
			Created:  img.Created.Unix(),
			Size:     img.SizeBytes,
		})
	}
	if result == nil {
		result = []ImageJSON{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleImagePull(w http.ResponseWriter, r *http.Request) {
	image := r.URL.Query().Get("fromImage")
	tag := r.URL.Query().Get("tag")
	if tag != "" && tag != "latest" {
		image = image + ":" + tag
	}
	if image == "" {
		writeError(w, http.StatusBadRequest, "fromImage is required")
		return
	}

	// Docker clients expect POST /images/create to stream NDJSON progress. The
	// engine's ImagePull blocks until the pull finishes, so we can't report
	// real per-layer progress; instead we run it in the background and emit a
	// periodic heartbeat status line so clients don't see an empty body and
	// time out. Fast failures (nonexistent image, auth) usually return before
	// the first heartbeat, letting us map them to a proper status code (404 /
	// 401) via writeRuntimeError. Once we've started streaming (200 committed)
	// an error can only be reported inside the stream as an errorDetail.
	repo := image
	if i := strings.LastIndex(repo, ":"); i >= 0 && !strings.Contains(repo[i:], "/") {
		repo = repo[:i]
	}

	done := make(chan error, 1)
	go func() { done <- s.eng.ImagePull(r.Context(), image, engine.ImagePullOpts{}) }()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	streaming := false
	var rc *http.ResponseController
	enc := json.NewEncoder(w)

	for {
		select {
		case err := <-done:
			if !streaming {
				if err != nil {
					writeRuntimeError(w, err, "image", image)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{
					"status": "Downloaded newer image for " + image,
				})
				s.publishEvent("image", "pull", image, map[string]string{"name": image})
				return
			}
			// Already streaming: report the outcome inside the stream.
			if err != nil {
				_ = enc.Encode(map[string]any{
					"error":       err.Error(),
					"errorDetail": map[string]string{"message": err.Error()},
				})
				_ = rc.Flush()
				return
			}
			_ = enc.Encode(map[string]string{"status": "Status: Downloaded newer image for " + image})
			_ = rc.Flush()
			s.publishEvent("image", "pull", image, map[string]string{"name": image})
			return
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !streaming {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				rc = http.NewResponseController(w)
				_ = enc.Encode(map[string]string{"status": "Pulling from " + repo})
				_ = rc.Flush()
				streaming = true
				continue
			}
			_ = enc.Encode(map[string]string{"status": "Downloading " + repo})
			_ = rc.Flush()
		}
	}
}

// imageRefMatches reports whether img matches the reference string ref.
// Docker clients send a mix of forms for the same image — short
// ("alpine:3"), repository-only ("nginx"), fully qualified with a default
// registry ("docker.io/library/alpine:3"), or by ID prefix — and expect
// all to resolve. Our in-VM parser currently flattens Name to the short
// form, so we have to both strip a qualified ref down to the short form
// and expand a short Name up to the qualified form when matching.
func imageRefMatches(img engine.ImageInfo, ref string) bool {
	full := img.Name + ":" + img.Tag
	if ref == img.ID || ref == img.Name || ref == full {
		return true
	}
	// Short-ID lookup: `docker inspect <12-char-id>` sends a prefix of the
	// image ID (optionally without the "sha256:" algorithm prefix). The old
	// code only did exact ID equality, so short IDs 404'd. Match on prefix.
	if id := strings.TrimPrefix(img.ID, "sha256:"); id != "" {
		r := strings.TrimPrefix(ref, "sha256:")
		if len(r) >= 3 && strings.HasPrefix(id, r) {
			return true
		}
	}
	candidates := []string{img.Name, full}
	// Expand a stored Name to its docker.io/* equivalent so a request that
	// includes the registry resolves. Two cases the in-VM parser flattens:
	//   "alpine"               → "docker.io/library/alpine"
	//   "tensorchord/pgvecto"  → "docker.io/tensorchord/pgvecto"
	// (third-party namespaces are NOT under /library/, so we have to try
	// docker.io/<as-is> too, not just docker.io/library/<as-is>).
	switch strings.Count(img.Name, "/") {
	case 0:
		candidates = append(candidates,
			"docker.io/library/"+img.Name,
			"docker.io/library/"+img.Name+":"+img.Tag,
		)
	case 1:
		candidates = append(candidates,
			"docker.io/"+img.Name,
			"docker.io/"+img.Name+":"+img.Tag,
		)
	}
	// Also contract the stored Name down to the short form for the
	// reverse case (request uses short, stored is qualified).
	for _, prefix := range []string{"docker.io/library/", "docker.io/"} {
		if short, ok := strings.CutPrefix(img.Name, prefix); ok {
			candidates = append(candidates, short, short+":"+img.Tag)
		}
	}
	return slices.Contains(candidates, ref)
}

func (s *Server) handleImageRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.eng.ImageRemove(r.Context(), name); err != nil {
		writeRuntimeError(w, err, "image", name)
		return
	}
	s.publishEvent("image", "delete", name, map[string]string{"name": name})
	writeJSON(w, http.StatusOK, []map[string]string{{"Deleted": name}})
}

func (s *Server) handleImageInspect(w http.ResponseWriter, r *http.Request) {
	// This handler is registered on the catch-all GET /images/{name...}, so it
	// also receives sub-resource paths like /images/foo/history. Only /json
	// (inspect) is implemented; route the rest to 501 instead of letting them
	// fall through to a confusing "image not found" 404. The image ref itself
	// may contain slashes (library/alpine), so key off the trailing segment.
	raw := r.PathValue("name")
	var name string
	switch {
	case raw == "history" || strings.HasSuffix(raw, "/history"):
		writeError(w, http.StatusNotImplemented, "image history is not implemented")
		return
	case strings.HasSuffix(raw, "/json"):
		name = strings.TrimSuffix(raw, "/json")
	default:
		name = raw
	}

	// Apple Container doesn't have a direct image inspect command
	// Return a minimal response
	images, err := s.eng.ImageList(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, img := range images {
		if imageRefMatches(img, name) {
			resp := map[string]any{
				"Id":       img.ID,
				"RepoTags": []string{img.Name + ":" + img.Tag},
				"Created":  img.Created.UTC().Format("2006-01-02T15:04:05Z"),
				"Size":     img.SizeBytes,
			}
			writeJSON(w, http.StatusOK, resp)
			return
		}
	}
	writeError(w, http.StatusNotFound, "image not found: "+name)
}
