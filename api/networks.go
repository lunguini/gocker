package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleNetworkList(w http.ResponseWriter, r *http.Request) {
	networks, err := s.eng.NetworkList(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var result []NetworkJSON
	for _, n := range networks {
		result = append(result, NetworkJSON{
			ID:     n.ID,
			Name:   n.Name,
			Driver: n.Driver,
			Scope:  n.Scope,
		})
	}
	if result == nil {
		result = []NetworkJSON{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleNetworkInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data, err := s.eng.NetworkInspect(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Server) handleNetworkCreate(w http.ResponseWriter, r *http.Request) {
	var req NetworkCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.eng.NetworkCreate(r.Context(), req.Name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"Id": req.Name})
}

func (s *Server) handleNetworkRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.eng.NetworkRemove(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNetworkConnect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req NetworkConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.eng.NetworkConnect(r.Context(), id, req.Container); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleNetworkDisconnect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req NetworkConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.eng.NetworkDisconnect(r.Context(), id, req.Container); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}
