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
	writeJSON(w, http.StatusCreated, VolumeJSON{Name: req.Name, Driver: req.Driver})
}

func (s *Server) handleVolumeRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.eng.VolumeRemove(r.Context(), name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleVolumeInspect(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	data, err := s.eng.VolumeInspect(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
