package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleVolumeList(w http.ResponseWriter, r *http.Request) {
	volumes, err := s.eng.VolumeList(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var result []*VolumeJSON
	for _, v := range volumes {
		result = append(result, &VolumeJSON{
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
		})
	}
	writeJSON(w, http.StatusOK, VolumeListResponse{Volumes: result})
}

func (s *Server) handleVolumeCreate(w http.ResponseWriter, r *http.Request) {
	var req VolumeCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.eng.VolumeCreate(r.Context(), req.Name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishEvent("volume", "create", req.Name, map[string]string{"driver": req.Driver})
	writeJSON(w, http.StatusCreated, VolumeJSON{Name: req.Name, Driver: req.Driver})
}

func (s *Server) handleVolumeRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.eng.VolumeRemove(r.Context(), name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishEvent("volume", "destroy", name, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleVolumeInspect(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	data, err := s.eng.VolumeInspect(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Apple CLI may return a JSON array with lowercase field names; Docker's
	// SDK expects a single object with capitalized fields.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		var arr []map[string]any
		if arrErr := json.Unmarshal(data, &arr); arrErr == nil {
			if len(arr) == 0 {
				writeError(w, http.StatusNotFound, "No such volume: "+name)
				return
			}
			raw = arr[0]
		} else {
			writeError(w, http.StatusInternalServerError, "failed to parse inspect data")
			return
		}
	}

	resolved := getString(raw, "name", "Name")
	if resolved == "" {
		resolved = name
	}
	resp := map[string]any{
		"Name":       resolved,
		"Driver":     getString(raw, "driver", "Driver"),
		"Mountpoint": getString(raw, "mountpoint", "Mountpoint", "source", "Source"),
		"CreatedAt":  getString(raw, "createdAt", "CreatedAt", "created", "Created"),
		"Scope":      "local",
		"Labels":     map[string]string{},
		"Options":    map[string]string{},
	}
	writeJSON(w, http.StatusOK, resp)
}
