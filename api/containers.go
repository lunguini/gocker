package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
)

var (
	execStore   = sync.Map{}
	execCounter atomic.Int64
)

func (s *Server) handleContainerList(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	containers, err := s.eng.ContainerList(r.Context(), all)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var result []ContainerJSON
	for _, c := range containers {
		result = append(result, ContainerJSON{
			ID:      c.ID,
			Names:   []string{"/" + c.Name},
			Image:   c.Image,
			Command: c.Command,
			Created: c.Created.Unix(),
			State:   c.State,
			Status:  c.Status,
			Ports:   []PortMapping{},
			NetworkSettings: &NetworkSettings{
				Networks: map[string]*EndpointSettings{
					"bridge": {IPAddress: c.IP},
				},
			},
		})
	}
	if result == nil {
		result = []ContainerJSON{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleContainerCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateContainerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	name := r.URL.Query().Get("name")
	var args []string
	args = append(args, "-d")
	if name != "" {
		args = append(args, "--name", name)
	}
	if req.Tty {
		args = append(args, "-t")
	}
	if req.OpenStdin {
		args = append(args, "-i")
	}
	if req.WorkingDir != "" {
		args = append(args, "-w", req.WorkingDir)
	}
	for _, env := range req.Env {
		args = append(args, "-e", env)
	}
	if req.HostConfig != nil {
		for _, bind := range req.HostConfig.Binds {
			args = append(args, "-v", bind)
		}
		if req.HostConfig.NetworkMode != "" {
			args = append(args, "--network", req.HostConfig.NetworkMode)
		}
	}
	args = append(args, req.Image)
	args = append(args, req.Cmd...)

	if err := s.eng.ContainerRun(r.Context(), args, false); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Try to find the container we just created to get its ID
	id := name
	if id == "" {
		id = "unknown"
	}
	writeJSON(w, http.StatusCreated, CreateContainerResponse{
		ID:       id,
		Warnings: []string{},
	})
}

func (s *Server) handleContainerStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.eng.ContainerStart(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.eng.ContainerStop(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerKill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.eng.ContainerStop(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	if err := s.eng.ContainerRemove(r.Context(), id, force); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data, err := s.eng.ContainerInspect(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Parse and re-wrap in Docker-compatible format
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		// Return raw if we can't parse
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
		return
	}

	// Build a Docker-compatible inspect response
	state := getString(raw, "state", "State", "status", "Status")
	running := state == "running"
	resp := map[string]any{
		"Id":    id,
		"Name":  "/" + getString(raw, "name", "Name"),
		"Image": getString(raw, "image", "Image"),
		"State": ContainerState{
			Status:  state,
			Running: running,
		},
		"Config": map[string]any{
			"Image": getString(raw, "image", "Image"),
			"Env":   []string{},
		},
		"HostConfig": map[string]any{
			"Binds":       []string{},
			"NetworkMode": "bridge",
		},
		"NetworkSettings": map[string]any{
			"Networks": map[string]any{
				"bridge": map[string]string{
					"IPAddress": getString(raw, "ip", "IP", "ipAddress", "IPAddress"),
				},
			},
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	follow := r.URL.Query().Get("follow") == "1" || r.URL.Query().Get("follow") == "true"

	if follow {
		stream, err := s.eng.ExecStream(r.Context(), "logs", id, "--follow")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer stream.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		buf := make([]byte, 4096)
		for {
			n, readErr := stream.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
			if readErr != nil {
				return
			}
		}
	}

	stdout, _, err := s.eng.Exec(r.Context(), "logs", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(stdout)
}

func (s *Server) handleExecCreate(w http.ResponseWriter, r *http.Request) {
	containerID := r.PathValue("id")
	var cfg ExecConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id := fmt.Sprintf("exec-%d", execCounter.Add(1))
	execStore.Store(id, execEntry{containerID: containerID, config: cfg})

	writeJSON(w, http.StatusCreated, ExecCreateResponse{ID: id})
}

func (s *Server) handleExecStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	val, ok := execStore.Load(id)
	if !ok {
		writeError(w, http.StatusNotFound, "exec not found")
		return
	}
	entry := val.(execEntry)
	execStore.Delete(id)

	stdout, _, err := s.eng.Exec(r.Context(), append([]string{"exec", entry.containerID}, entry.config.Cmd...)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(stdout)
}

type execEntry struct {
	containerID string
	config      ExecConfig
}

func getString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}
