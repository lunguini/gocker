package api

import (
	"fmt"
	"net/http"
	"strings"

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
			Size:     0,
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

	if err := s.eng.ImagePull(r.Context(), image, engine.ImagePullOpts{}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.publishEvent("image", "pull", image, map[string]string{"name": image})
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"status":"Downloaded newer image for %s"}`, image)
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
		if short := strings.TrimPrefix(img.Name, prefix); short != img.Name {
			candidates = append(candidates, short, short+":"+img.Tag)
		}
	}
	for _, c := range candidates {
		if ref == c {
			return true
		}
	}
	return false
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
	// Apple Container doesn't have a direct image inspect command
	// Return a minimal response
	name := strings.TrimSuffix(r.PathValue("name"), "/json")
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
				"Size":     0,
			}
			writeJSON(w, http.StatusOK, resp)
			return
		}
	}
	writeError(w, http.StatusNotFound, "image not found: "+name)
}
