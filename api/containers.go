package api

import (
	"encoding/json"
	"fmt"
	"io"
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
		// Docker CLI sends NetworkMode="default" on every `docker run` without
		// an explicit --network; that's Docker Engine's internal sentinel
		// meaning "use the default network". The backend CLIs (Apple
		// container, nerdctl) don't recognize "default" — pass nothing and
		// let them pick their own default.
		if req.HostConfig.NetworkMode != "" && req.HostConfig.NetworkMode != "default" {
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
	s.publishEvent("container", "create", id, map[string]string{"image": req.Image, "name": name})
	s.publishEvent("container", "start", id, map[string]string{"image": req.Image, "name": name})
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
	s.publishEvent("container", "start", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.eng.ContainerStop(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishEvent("container", "stop", id, nil)
	s.publishEvent("container", "die", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerKill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.eng.ContainerStop(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishEvent("container", "kill", id, nil)
	s.publishEvent("container", "die", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	if err := s.eng.ContainerRemove(r.Context(), id, force); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishEvent("container", "destroy", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data, err := s.eng.ContainerInspect(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Parse and re-wrap in Docker-compatible format.
	// Apple CLI may return a JSON array — unwrap the first element.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		var arr []map[string]any
		if arrErr := json.Unmarshal(data, &arr); arrErr == nil {
			if len(arr) == 0 {
				writeError(w, http.StatusNotFound, "No such container: "+id)
				return
			}
			raw = arr[0]
		} else {
			writeError(w, http.StatusInternalServerError, "failed to parse inspect data")
			return
		}
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
		defer func() { _ = stream.Close() }()
		w.Header().Set("Content-Type", "application/octet-stream")
		buf := make([]byte, 4096)
		for {
			n, readErr := stream.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
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
	_, _ = w.Write(stdout)
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
	// Mark the exec as running so /exec/{id}/json reports a sensible shape
	// mid-flight if the Docker CLI inspects before the stream finishes.
	entry.running = true
	execStore.Store(id, entry)
	// When the handler returns, persist the final state (running=false,
	// exit code) so /exec/{id}/json can answer correctly.
	defer func() {
		execStore.Store(id, entry)
	}()

	// Parse start-time request. Detach=true is a fire-and-forget; all other
	// shapes require a hijacked bidirectional stream (Docker CLI upgrades
	// from HTTP to a raw TCP stream).
	var req ExecStartRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	// Tty can be set at create OR start time; the start value wins if present.
	tty := entry.config.Tty || req.Tty

	if req.Detach {
		// Non-interactive background run — collect output, discard, return 200.
		_, _, err := s.eng.Exec(r.Context(), append([]string{"exec", entry.containerID}, entry.config.Cmd...)...)
		entry.running = false
		if err != nil {
			entry.exitCode = 1
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		entry.exitCode = 0
		w.WriteHeader(http.StatusOK)
		return
	}

	// Hijack the connection so we can write a raw bidirectional stream.
	// ResponseController follows Unwrap() on wrapped writers (our logging
	// middleware wraps every response) so hijacking still works.
	conn, buf, err := http.NewResponseController(w).Hijack()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hijack: "+err.Error())
		return
	}
	defer func() { _ = conn.Close() }()

	// Docker CLI sends 'Upgrade: tcp' and expects a 101 Switching Protocols
	// response before switching to the raw stream. Dockerd's message is
	// literally "UPGRADED"; matching that keeps the CLI happy.
	if _, err := fmt.Fprintf(buf, "HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n"); err != nil {
		return
	}
	_ = buf.Flush()

	stream, err := s.eng.ExecStream(r.Context(), append([]string{"exec", entry.containerID}, entry.config.Cmd...)...)
	if err != nil {
		// Can't write an error cleanly at this point (101 already flushed).
		// Just close the connection and mark as failed.
		entry.running = false
		entry.exitCode = 1
		return
	}

	if tty {
		// TTY mode: stdout/stderr merged, pass bytes through raw.
		_, _ = io.Copy(conn, stream)
	} else {
		// Non-TTY multiplex: 8-byte frame header per chunk.
		//   [0]     = stream type (1=stdout, 2=stderr). We only have stdout
		//             since ExecStream merges stderr into the server's stderr.
		//   [1-3]   = reserved zeros
		//   [4-7]   = big-endian uint32 payload size
		writeFramedChunks(conn, stream)
	}

	// Close() waits for the command to finish and returns its exit error.
	// We capture the exit code so /exec/{id}/json can report it to clients
	// that check `docker exec`'s exit status (most of them do).
	closeErr := stream.Close()
	entry.running = false
	entry.exitCode = exitCodeFromError(closeErr)
}

// exitCodeFromError extracts a shell-style exit code from exec.Cmd.Wait()'s
// error. nil → 0. *exec.ExitError → its ExitCode(). Anything else (e.g.
// context cancellation, pipe failure) → 1.
func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	type exitCoder interface{ ExitCode() int }
	if ec, ok := err.(exitCoder); ok {
		return ec.ExitCode()
	}
	return 1
}

// handleExecInspect implements GET /exec/{id}/json. Docker CLI calls it
// after the stream finishes to report the command's exit code. The shape
// mirrors docker's types.ContainerExecInspect.
func (s *Server) handleExecInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	val, ok := execStore.Load(id)
	if !ok {
		writeError(w, http.StatusNotFound, "exec not found")
		return
	}
	entry := val.(execEntry)
	writeJSON(w, http.StatusOK, map[string]any{
		"ID":            id,
		"ExecID":        id,
		"ContainerID":   entry.containerID,
		"Running":       entry.running,
		"ExitCode":      entry.exitCode,
		"ProcessConfig": map[string]any{},
		"OpenStdin":     entry.config.AttachStdin,
		"OpenStdout":    entry.config.AttachStdout,
		"OpenStderr":    entry.config.AttachStderr,
		"CanRemove":     !entry.running,
	})
}

// writeFramedChunks reads from src in 4KB chunks and writes Docker's 8-byte
// multiplex frame header followed by the payload. All output is tagged as
// stdout (stream type 1) because our ExecStream doesn't split stderr today.
func writeFramedChunks(dst io.Writer, src io.Reader) {
	buf := make([]byte, 4096)
	header := make([]byte, 8)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			header[0] = 1 // stdout
			header[1] = 0
			header[2] = 0
			header[3] = 0
			header[4] = byte(n >> 24)
			header[5] = byte(n >> 16)
			header[6] = byte(n >> 8)
			header[7] = byte(n)
			if _, werr := dst.Write(header); werr != nil {
				return
			}
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

type execEntry struct {
	containerID string
	config      ExecConfig
	running     bool
	exitCode    int
}

func getString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

// extractStringMap pulls a map[string]string out of a raw inspect payload,
// trying each candidate key (for the case-insensitive Apple CLI / nerdctl
// naming split) and returning an empty (non-nil) map if nothing matches.
// Non-string values are rendered via %v so the wire shape still satisfies
// Docker SDK's strict decoders.
func extractStringMap(m map[string]any, keys ...string) map[string]string {
	for _, k := range keys {
		raw, ok := m[k]
		if !ok {
			continue
		}
		if mm, ok := raw.(map[string]any); ok {
			out := make(map[string]string, len(mm))
			for k2, v := range mm {
				out[k2] = fmt.Sprintf("%v", v)
			}
			return out
		}
	}
	return map[string]string{}
}

// extractLabels is extractStringMap pinned to the "labels" / "Labels" keys,
// with a fallback to `config.labels` which is where Apple's `container
// network inspect` nests them. Compose relies on labels being passed
// through verbatim to decide whether a network/volume is "its own" vs
// foreign — returning an empty map causes compose to refuse its own
// resources with "not created by Docker Compose, use external: true".
func extractLabels(m map[string]any) map[string]string {
	if labels := extractStringMap(m, "labels", "Labels"); len(labels) > 0 {
		return labels
	}
	// Apple CLI: labels live under config.labels in network inspect output.
	for _, nestedKey := range []string{"config", "Config"} {
		if nested, ok := m[nestedKey].(map[string]any); ok {
			if labels := extractStringMap(nested, "labels", "Labels"); len(labels) > 0 {
				return labels
			}
		}
	}
	return map[string]string{}
}
