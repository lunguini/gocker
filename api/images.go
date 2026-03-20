package api

import (
	"fmt"
	"net/http"
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

	if err := s.eng.ImagePull(r.Context(), image); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"Downloaded newer image for %s"}`, image)
}

func (s *Server) handleImageRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.eng.ImageRemove(r.Context(), name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, []map[string]string{{"Deleted": name}})
}

func (s *Server) handleImageInspect(w http.ResponseWriter, r *http.Request) {
	// Apple Container doesn't have a direct image inspect command
	// Return a minimal response
	name := r.PathValue("name")
	images, err := s.eng.ImageList(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, img := range images {
		fullName := img.Name + ":" + img.Tag
		if img.Name == name || fullName == name || img.ID == name {
			resp := map[string]any{
				"Id":       img.ID,
				"RepoTags": []string{fullName},
				"Created":  img.Created.Unix(),
				"Size":     0,
			}
			writeJSON(w, http.StatusOK, resp)
			return
		}
	}
	writeError(w, http.StatusNotFound, "image not found: "+name)
}
