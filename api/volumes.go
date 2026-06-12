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
		labels := v.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		result = append(result, &VolumeJSON{
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
			Labels:     labels,
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
		writeRuntimeError(w, err, "volume", name)
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
	// Reshape into the real Docker SDK volume.Volume with every map/field
	// non-nil. See api/inspect.go.
	v, err := reshapeVolumeInspect(data, name)
	if err != nil {
		writeError(w, http.StatusNotFound, "No such volume: "+name)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
