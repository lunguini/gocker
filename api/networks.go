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

	// Apple CLI returns a JSON array with lowercase field names; Docker's SDK
	// expects a single object with capitalized fields.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		var arr []map[string]any
		if arrErr := json.Unmarshal(data, &arr); arrErr == nil {
			if len(arr) == 0 {
				writeError(w, http.StatusNotFound, "No such network: "+id)
				return
			}
			raw = arr[0]
		} else {
			writeError(w, http.StatusInternalServerError, "failed to parse inspect data")
			return
		}
	}

	name := getString(raw, "name", "Name")
	resp := map[string]any{
		"Id":         getString(raw, "id", "ID", "Id"),
		"Name":       name,
		"Scope":      getString(raw, "scope", "Scope"),
		"Driver":     getString(raw, "driver", "Driver"),
		"EnableIPv6": false,
		"Internal":   false,
		"Attachable": false,
		"Ingress":    false,
		"IPAM": map[string]any{
			"Driver":  "default",
			"Options": map[string]string{},
			"Config":  []any{},
		},
		"ConfigFrom": map[string]any{"Network": ""},
		"ConfigOnly": false,
		"Containers": map[string]any{},
		"Options":    map[string]string{},
		"Labels":     map[string]string{},
	}
	writeJSON(w, http.StatusOK, resp)
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
	s.publishEvent("network", "create", req.Name, map[string]string{"name": req.Name})
	writeJSON(w, http.StatusCreated, map[string]string{"Id": req.Name})
}

func (s *Server) handleNetworkRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.eng.NetworkRemove(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishEvent("network", "destroy", id, nil)
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
	s.publishEvent("network", "connect", id, map[string]string{"container": req.Container})
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
	s.publishEvent("network", "disconnect", id, map[string]string{"container": req.Container})
	w.WriteHeader(http.StatusOK)
}
