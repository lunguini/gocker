package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lunguini/gocker/engine"
)

var (
	execStore   = sync.Map{}
	execCounter atomic.Int64
)

func (s *Server) handleContainerList(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"

	// Compose v2 drives 'docker compose exec SERVICE' off this filter — it
	// asks /containers/json with label constraints identifying the
	// project+service and picks the first match. Returning everything makes
	// compose operate on the wrong container (client instead of server).
	filt, ferr := parseListFilters(r.URL.Query().Get("filters"))
	if ferr != nil {
		writeError(w, http.StatusBadRequest, "invalid filters: "+ferr.Error())
		return
	}
	// A status filter implies Docker returns matching containers regardless of
	// the `all` param (e.g. `docker ps -f status=exited` without `-a`). Fetch
	// the full set so the filter can find stopped/created containers.
	if len(filt.statuses) > 0 {
		all = true
	}

	containers, err := s.eng.ContainerList(r.Context(), all)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := []ContainerJSON{}
	for _, c := range containers {
		if !filt.match(c) {
			continue
		}
		// Docker's Created is a Unix timestamp. When the backend didn't give
		// us a real creation time, c.Created is the zero time and .Unix()
		// yields -62135596800 — clients (lazydocker) then render "created
		// 2000 years ago". Emit 0 instead, which clients treat as "unknown".
		created := int64(0)
		if !c.Created.IsZero() {
			created = c.Created.Unix()
		}
		cj := ContainerJSON{
			ID:      c.ID,
			Names:   []string{"/" + c.Name},
			Image:   c.Image,
			Command: c.Command,
			Created: created,
			State:   deriveContainerState(c.State, c.Status),
			Status:  c.Status,
			Ports:   parseNerdctlPorts(c.Ports),
			Labels:  c.Labels,
		}
		// Only report network settings when we actually have an address.
		// Previously every container was fabricated onto a "bridge" network
		// with an empty IP, which is dishonest and confuses clients that read
		// NetworkSettings.Networks. We still don't know the real network name
		// from the list output, so use "bridge" only as the endpoint key when
		// an IP exists.
		if c.IP != "" {
			cj.NetworkSettings = &NetworkSettings{
				Networks: map[string]*EndpointSettings{
					"bridge": {IPAddress: c.IP},
				},
			}
		}
		result = append(result, cj)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleContainerCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateContainerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// ext4-formatted volumes (Apple Container CLI's default) have a
	// lost+found directory at the root which breaks initdb-style data-dir
	// setup for Postgres/MySQL. Docker's ext4 volumes don't have this.
	// Inject the appropriate env var pointing at a subdirectory — users
	// running `docker compose up` against gocker shouldn't have to know
	// about this themselves.
	req.Env = applyInitDirWorkarounds(req.Image, req.HostConfig, req.Env)

	name := r.URL.Query().Get("name")
	// Build the create arg vector. This is the same translation the old
	// run-at-create handler did, minus `-d` — the container is created
	// stopped and only started by a later POST /containers/{id}/start (the
	// Docker create→start choreography). Publishing ports (previously dropped
	// silently, finding C4) happens here via portPublishArgs.
	var args []string
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
	// Compose v2 identifies containers it owns by label
	// (com.docker.compose.{project,service,version}). Without these,
	// `docker compose ps` leaves the Service column empty and
	// `docker compose down` can't find its own containers.
	for _, k := range sortedKeys(req.Labels) {
		args = append(args, "--label", k+"="+req.Labels[k])
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
	args = append(args, portPublishArgs(req.HostConfig, req.ExposedPorts)...)
	// Compose v2 (and Docker SDK) sends Entrypoint as a separate field; we
	// were silently dropping it, so containers ran the image's default
	// CMD instead of the user-supplied entrypoint and exited immediately
	// (alpine's /bin/sh without args returns 0). nerdctl/Apple's
	// --entrypoint takes a single string; if compose sends a multi-element
	// list, the first element is the entrypoint binary and the rest are
	// arguments that have to come AFTER the image, joined with Cmd.
	if len(req.Entrypoint) > 0 {
		args = append(args, "--entrypoint", req.Entrypoint[0])
		// Tail elements become the leading positional args (before Cmd).
		// We accumulate them into a separate slice that's flushed below.
		req.Cmd = append(append([]string{}, req.Entrypoint[1:]...), req.Cmd...)
	}
	args = append(args, req.Image)
	// Guard the user's Cmd against flag reparsing in downstream CLIs. The
	// inner `gocker run` (or nerdctl) consumes this arg list; if Cmd begins
	// with or contains things like '-c' (literally the Docker API convention
	// for `sh -c '...'`), a flag parser will happily steal it. Docker's own
	// CLI stops at the image positional; we can't count on urfave/cli doing
	// the same, so emit an explicit `--` separator.
	if len(req.Cmd) > 0 {
		args = append(args, "--")
		args = append(args, req.Cmd...)
	}

	// Create the container WITHOUT starting it. The backend prints the new
	// container's real ID on stdout, which we return directly — no more
	// guessing from the ?name= param or resolving by list.
	id, err := s.eng.ContainerCreate(r.Context(), args)
	if err != nil {
		// A missing image maps to 404 ("No such image"); real backend failures
		// to 500 — matching Docker's create semantics.
		writeRuntimeError(w, err, "image", req.Image)
		return
	}
	// Some backends/shapes may not echo the ID (or echo a truncated form);
	// fall back to resolving by name so the response is still addressable.
	if id == "" {
		id = s.resolveContainerID(r.Context(), name)
	}

	warnings := unsupportedFieldWarnings(&req)

	// Create only publishes a `create` event now — the container isn't
	// started here, so the `start` event belongs to POST /containers/{id}/start.
	if id != "" {
		s.publishEvent("container", "create", id, map[string]string{"image": req.Image, "name": name})
	}
	// The response Id must be non-empty for clients; fall back to the name when
	// the backend didn't echo an ID and the list lookup missed.
	respID := id
	if respID == "" {
		respID = name
	}
	writeJSON(w, http.StatusCreated, CreateContainerResponse{
		ID:       respID,
		Warnings: warnings,
	})
}

// portPublishArgs translates Docker's HostConfig.PortBindings and
// ExposedPorts into `-p`/`--expose` CLI flags. PortBindings (published ports)
// become `-p [hostIP:]hostPort:containerPort/proto`; an empty HostPort lets the
// backend pick an ephemeral host port. ExposedPorts that aren't also published
// become `--expose containerPort/proto` (documented, not published) — matching
// Docker. Previously both were dropped, so `ports:` in a compose file and `-p`
// over the API did nothing (finding C4). Ordering is deterministic (sorted
// keys) so create commands are reproducible.
func portPublishArgs(hc *HostConfig, exposed map[string]struct{}) []string {
	var args []string
	bound := map[string]bool{}
	if hc != nil {
		for _, portProto := range sortedPortKeys(hc.PortBindings) {
			bound[portProto] = true
			cp := normalizePortProto(portProto)
			for _, b := range hc.PortBindings[portProto] {
				spec := ""
				if b.HostIP != "" {
					spec = b.HostIP + ":"
				}
				if b.HostPort != "" {
					spec += b.HostPort + ":"
				}
				spec += cp
				args = append(args, "-p", spec)
			}
		}
	}
	for _, portProto := range sortedExposedKeys(exposed) {
		if bound[portProto] {
			continue
		}
		args = append(args, "--expose", normalizePortProto(portProto))
	}
	return args
}

// normalizePortProto returns "port/proto", defaulting the proto to tcp when
// Docker sent a bare "80" key. Keeping the /proto suffix explicit is accepted
// by both nerdctl and Apple's container CLI.
func normalizePortProto(portProto string) string {
	if strings.Contains(portProto, "/") {
		return portProto
	}
	return portProto + "/tcp"
}

func sortedPortKeys(m map[string][]PortBind) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedExposedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// unsupportedFieldWarnings reports, via the create response's Warnings array,
// the client-supplied fields gocker knowingly drops rather than silently
// ignoring them (Docker uses Warnings for exactly this). These are decoded but
// not forwarded to the backend — see the HostConfig struct comment.
func unsupportedFieldWarnings(req *CreateContainerRequest) []string {
	warnings := []string{}
	if req.User != "" {
		warnings = append(warnings, "User is not applied: Apple's container exec/run has no --user; set the user in the image instead")
	}
	if hc := req.HostConfig; hc != nil {
		if hc.Memory > 0 {
			warnings = append(warnings, "HostConfig.Memory is not applied by gocker")
		}
		if len(hc.CapAdd) > 0 {
			warnings = append(warnings, "HostConfig.CapAdd is not applied by gocker")
		}
		if len(hc.ExtraHosts) > 0 {
			warnings = append(warnings, "HostConfig.ExtraHosts is not applied by gocker")
		}
		if hc.RestartPolicy != nil && hc.RestartPolicy.Name != "" && hc.RestartPolicy.Name != "no" {
			warnings = append(warnings, "HostConfig.RestartPolicy is not applied by gocker")
		}
	}
	return warnings
}

// resolveContainerID looks up the real container ID for a just-created
// container by matching its name against the list output. Returns "" when the
// name is empty or no match is found (transient list failures included).
func (s *Server) resolveContainerID(ctx context.Context, name string) string {
	if name == "" {
		return ""
	}
	containers, err := s.eng.ContainerList(ctx, true)
	if err != nil {
		return ""
	}
	want := strings.TrimPrefix(name, "/")
	for _, c := range containers {
		if strings.TrimPrefix(c.Name, "/") == want {
			return c.ID
		}
	}
	return ""
}

func (s *Server) handleContainerStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.eng.ContainerStart(r.Context(), id); err != nil {
		writeRuntimeError(w, err, "container", id)
		return
	}
	s.publishEvent("container", "start", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.eng.ContainerStop(r.Context(), id); err != nil {
		writeRuntimeError(w, err, "container", id)
		return
	}
	s.publishEvent("container", "stop", id, nil)
	s.publishEvent("container", "die", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerKill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.eng.ContainerStop(r.Context(), id); err != nil {
		writeRuntimeError(w, err, "container", id)
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
		writeRuntimeError(w, err, "container", id)
		return
	}
	s.publishEvent("container", "destroy", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data, err := s.eng.ContainerInspect(r.Context(), id)
	if err != nil {
		// Map "no such container" to 404 but genuine backend failures to 500,
		// like the sibling handlers — a blanket 404 masked real errors.
		writeRuntimeError(w, err, "container", id)
		return
	}
	// Reshape into the real Docker SDK ContainerJSON type with every
	// pointer/slice/map field guaranteed non-nil. See api/inspect.go for
	// the rationale — we previously did ad-hoc map-patching and kept
	// missing fields that individual clients dereferenced.
	c, err := reshapeContainerInspect(data)
	if err != nil {
		writeError(w, http.StatusNotFound, "No such container: "+id)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	q := r.URL.Query()
	opts := engine.LogsOptions{
		Follow:     q.Get("follow") == "1" || q.Get("follow") == "true",
		Tail:       q.Get("tail"),
		Since:      q.Get("since"),
		Until:      q.Get("until"),
		Timestamps: q.Get("timestamps") == "1" || q.Get("timestamps") == "true",
	}
	// Docker uses "0" / unset Unix epoch to mean "from the beginning". The
	// underlying CLIs treat "0" as a literal timestamp and return nothing —
	// drop it so they default to full backlog.
	if opts.Since == "0" || opts.Since == "0.000000000" {
		opts.Since = ""
	}
	if opts.Until == "0" || opts.Until == "0.000000000" {
		opts.Until = ""
	}

	// Docker clients (including lazydocker) expect logs to be multiplexed
	// with the same 8-byte frame header as /exec/{id}/start when the
	// container has no TTY. The client decodes frames and crashes or shows
	// garbage on raw-byte input. Emit framed output by default.
	w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")

	// If follow=1 on a stopped container, downgrade to non-follow. Docker
	// clients ask for follow by default but expect it to return and exit
	// when the container isn't running; our backing `logs --follow` hangs
	// waiting for new output instead.
	if opts.Follow {
		if state, err := s.containerState(r.Context(), id); err == nil && state != "running" {
			opts.Follow = false
		}
	}

	args := append([]string{"logs"}, engine.LogsFlags(opts)...)
	args = append(args, id)

	wantStdout := q.Get("stdout") != "0" && q.Get("stdout") != "false"
	wantStderr := q.Get("stderr") != "0" && q.Get("stderr") != "false"
	// Defaults: if neither was specified, Docker returns nothing. Most
	// clients pass at least one. Be liberal — if both unset, return both.
	if q.Get("stdout") == "" && q.Get("stderr") == "" {
		wantStdout, wantStderr = true, true
	}

	// Follow and non-follow take the same path: ExecStreamSplit runs the
	// backing `logs` command (with or without --follow already baked into
	// args) and streamFramedLogs pumps until EOF. The follow flag only
	// changes whether that command blocks for more output.
	stdout, stderr, err := s.eng.ExecStreamSplit(r.Context(), args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	streamFramedLogs(w, stdout, stderr, wantStdout, wantStderr)
}

// containerState returns the current state string (e.g. "running",
// "exited", "stopped") for a container, by reading its inspect payload.
// Returns an error only on transport/parse failure — unknown state is OK.
func (s *Server) containerState(ctx context.Context, id string) (string, error) {
	data, err := s.eng.ContainerInspect(ctx, id)
	if err != nil {
		return "", err
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		var arr []map[string]any
		if arrErr := json.Unmarshal(data, &arr); arrErr != nil || len(arr) == 0 {
			return "", fmt.Errorf("parse inspect: %w", err)
		}
		obj = arr[0]
	}
	if state, ok := obj["State"].(map[string]any); ok {
		if s, ok := state["Status"].(string); ok {
			return strings.ToLower(s), nil
		}
	}
	// Fallback for flat Apple CLI inspects.
	if s, ok := obj["status"].(string); ok {
		return strings.ToLower(s), nil
	}
	if s, ok := obj["State"].(string); ok {
		return strings.ToLower(s), nil
	}
	return "", nil
}

func (s *Server) handleExecCreate(w http.ResponseWriter, r *http.Request) {
	containerID := r.PathValue("id")
	var cfg ExecConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	pruneExecStore()

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

	// Parse start-time request. Detach=true is a fire-and-forget; all other
	// shapes require a hijacked bidirectional stream (Docker CLI upgrades
	// from HTTP to a raw TCP stream).
	var req ExecStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "malformed exec start request: "+err.Error())
		return
	}
	// Tty can be set at create OR start time; the start value wins if present.
	tty := entry.config.Tty || req.Tty
	execArgs := buildExecArgs(entry, tty)

	if req.Detach {
		// Detached run: fire-and-forget in the background. Use a background
		// context (not r.Context()) so the exec survives the HTTP client
		// disconnecting — a detached exec must outlive its request. Respond
		// 200 immediately, as Docker does, and record the result when it
		// finishes for a later /exec/{id}/json.
		go func() {
			_, _, err := s.eng.Exec(context.Background(), execArgs...)
			finishExec(id, exitCodeFromError(err))
		}()
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

	// True `-t` PTY allocation is still unavailable (the outer shared-VM exec
	// can't allocate a pty without a real host terminal — CLAUDE.md's TTY
	// rules), so the TTY path raw-copies merged output and doesn't pipe stdin.
	if tty {
		// Client asked for a merged raw stream (TTY semantics). ExecStream
		// merges the child's stderr into the server's stderr, so we pass its
		// stdout through unframed, matching what a TTY client expects.
		stream, serr := s.eng.ExecStream(r.Context(), execArgs...)
		if serr != nil {
			finishExec(id, 1)
			return
		}
		_, _ = io.Copy(conn, stream)
		finishExec(id, exitCodeFromError(stream.Close()))
		return
	}

	// Non-TTY: wire the hijacked connection's read side to the process stdin
	// (real `docker exec -i` input piping) via ExecStreamStdin, and frame
	// stdout/stderr separately with their correct Docker multiplex stream
	// types (1=stdout, 2=stderr). `buf` reads any bytes already buffered by
	// the hijack plus the live connection; when the client half-closes its
	// write side, buf hits EOF, the backend closes the child's stdin, and
	// stdin-reading commands like `cat` terminate. Only attach stdin when the
	// client actually requested it (AttachStdin) — otherwise leave it nil so a
	// non-`-i` exec isn't blocked reading a connection that never sends input.
	var stdinR io.Reader
	if entry.config.AttachStdin {
		stdinR = buf
	}
	stdout, stderr, serr := s.eng.ExecStreamStdin(r.Context(), stdinR, execArgs...)
	if serr != nil {
		finishExec(id, 1)
		return
	}
	closeErr := streamFramedExecSplit(conn, stdout, stderr)
	finishExec(id, exitCodeFromError(closeErr))
}

// buildExecArgs assembles the backend exec argument vector for an exec entry.
// Env/WorkingDir/User are forwarded as -e/-w/-u where the backend supports
// them (nerdctl fully; Apple's `container exec` has no --user, so a User-set
// exec surfaces a clear backend error there rather than being silently
// dropped). The `-i` keeps stdin open on the backend side; see the stdin
// limitation note in handleExecStart.
func buildExecArgs(entry execEntry, tty bool) []string {
	args := []string{"exec", "-i"}
	if entry.config.WorkingDir != "" {
		args = append(args, "-w", entry.config.WorkingDir)
	}
	if entry.config.User != "" {
		args = append(args, "-u", entry.config.User)
	}
	for _, e := range entry.config.Env {
		args = append(args, "-e", e)
	}
	args = append(args, entry.containerID)
	args = append(args, entry.config.Cmd...)
	return args
}

// finishExec records an exec's terminal state (running=false, exit code,
// finish time) so /exec/{id}/json can report it and pruneExecStore can later
// evict it. Safe to call from the request goroutine or a detached one.
func finishExec(id string, exitCode int) {
	val, ok := execStore.Load(id)
	if !ok {
		return
	}
	entry := val.(execEntry)
	entry.running = false
	entry.exitCode = exitCode
	entry.finishedAt = time.Now()
	execStore.Store(id, entry)
}

// streamFramedExecSplit pumps stdout/stderr concurrently into Docker's 8-byte
// multiplex frames (type 1 for stdout, 2 for stderr) written to dst, then
// closes both readers and returns the process exit error (the last reader
// closed reaps the child). Unlike streamFramedLogs it returns that error so
// the caller can surface the exit code.
func streamFramedExecSplit(dst io.Writer, stdout, stderr io.ReadCloser) error {
	var mu sync.Mutex
	var wg sync.WaitGroup
	pump := func(src io.Reader, streamType byte) {
		defer wg.Done()
		if src == nil {
			return
		}
		buf := make([]byte, 4096)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				mu.Lock()
				writeFrameHeader(dst, streamType, n)
				_, werr := dst.Write(buf[:n])
				mu.Unlock()
				if werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}
	wg.Add(2)
	go pump(stdout, 1)
	go pump(stderr, 2)
	wg.Wait()
	// Close both; closeOne reaps on the last close and returns the exit error.
	var err1, err2 error
	if stdout != nil {
		err1 = stdout.Close()
	}
	if stderr != nil {
		err2 = stderr.Close()
	}
	if err1 != nil {
		return err1
	}
	return err2
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

// handleContainerStats implements GET /containers/{id}/stats. Docker's
// shape is types.StatsJSON; clients like lazydocker poll this for per-
// second CPU/mem/net/IO counters. We don't have a real source for these
// in Apple Container CLI today, so return a well-formed zero-value stream
// (or single snapshot when stream=0) — that keeps lazydocker's stats
// panel alive instead of showing "endpoint not found".
//
// TODO: populate real counters by probing /sys/fs/cgroup inside the VM or
// shelling out to `nerdctl stats` (issue #TBD for proper implementation).
func (s *Server) handleContainerStats(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	stream := r.URL.Query().Get("stream") != "0" && r.URL.Query().Get("stream") != "false"

	// Verify the container exists so we don't pretend to stream stats for
	// something imaginary — clients treat 404 here as a clean signal.
	if _, err := s.eng.ContainerInspect(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// Flush via ResponseController so it works through the logging middleware's
	// wrapper (which doesn't implement http.Flusher). A direct
	// w.(http.Flusher) assertion fails there and the stats stream never
	// reaches the client — lazydocker's stats panel hangs.
	rc := http.NewResponseController(w)

	writeSnapshot := func() bool {
		snap := zeroStatsJSON(id)
		data, _ := json.Marshal(snap)
		data = append(data, '\n')
		if _, err := w.Write(data); err != nil {
			return false
		}
		_ = rc.Flush()
		return true
	}

	if !writeSnapshot() {
		return
	}
	if !stream {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !writeSnapshot() {
				return
			}
		}
	}
}

// zeroStatsJSON returns a StatsJSON-shaped zero-value snapshot with the
// current timestamps. Enough for clients to render "0% CPU, 0B memory"
// without erroring on missing fields.
func zeroStatsJSON(id string) map[string]any {
	now := time.Now()
	zero := func() map[string]any { return map[string]any{} }
	cpu := map[string]any{
		"cpu_usage": map[string]any{
			"total_usage":         0,
			"percpu_usage":        []int{0},
			"usage_in_kernelmode": 0,
			"usage_in_usermode":   0,
		},
		"system_cpu_usage": 0,
		"online_cpus":      1,
		"throttling_data": map[string]any{
			"periods":           0,
			"throttled_periods": 0,
			"throttled_time":    0,
		},
	}
	return map[string]any{
		"id":            id,
		"name":          "/" + id,
		"read":          now.UTC().Format(time.RFC3339Nano),
		"preread":       now.Add(-time.Second).UTC().Format(time.RFC3339Nano),
		"num_procs":     0,
		"cpu_stats":     cpu,
		"precpu_stats":  cpu,
		"memory_stats":  map[string]any{"usage": 0, "limit": 0, "max_usage": 0, "stats": zero()},
		"pids_stats":    map[string]any{"current": 0},
		"networks":      zero(),
		"blkio_stats":   zero(),
		"storage_stats": zero(),
	}
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

// streamFramedLogs concurrently consumes stdout/stderr pipes from a logs
// command, writing each chunk into the Docker multiplex frame format
// (header[0]=1 for stdout, 2 for stderr) so clients like lazydocker can
// demultiplex them. Selectively suppresses streams the client opted out of.
func streamFramedLogs(w http.ResponseWriter, stdout io.ReadCloser, stderr io.ReadCloser, wantStdout, wantStderr bool) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	// Use ResponseController so flushing works through middleware wrappers
	// like loggingResponseWriter that don't themselves implement Flusher.
	rc := http.NewResponseController(w)

	pump := func(src io.ReadCloser, streamType byte, want bool) {
		defer wg.Done()
		if src == nil {
			return
		}
		defer func() { _ = src.Close() }()
		buf := make([]byte, 4096)
		for {
			n, err := src.Read(buf)
			// Always read both pipes so the producer doesn't block, but
			// only frame the streams the client asked for.
			if n > 0 && want {
				mu.Lock()
				writeFrameHeader(w, streamType, n)
				_, werr := w.Write(buf[:n])
				_ = rc.Flush()
				mu.Unlock()
				if werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}
	wg.Add(2)
	go pump(stdout, 1, wantStdout)
	go pump(stderr, 2, wantStderr)
	wg.Wait()
}

func writeFrameHeader(w io.Writer, streamType byte, n int) {
	header := []byte{streamType, 0, 0, 0,
		byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	_, _ = w.Write(header)
}

type execEntry struct {
	containerID string
	config      ExecConfig
	running     bool
	exitCode    int
	finishedAt  time.Time
}

// execStoreTTL bounds how long a finished exec entry is retained for a later
// /exec/{id}/json inspect. Without eviction the store grows unbounded in a
// long-lived daemon under compose healthchecks that exec every few seconds.
const execStoreTTL = 10 * time.Minute

// pruneExecStore evicts finished exec entries older than execStoreTTL. Called
// on each exec create so the sweep cost is amortized and the map stays bounded.
func pruneExecStore() {
	now := time.Now()
	execStore.Range(func(k, v any) bool {
		e := v.(execEntry)
		if !e.running && !e.finishedAt.IsZero() && now.Sub(e.finishedAt) > execStoreTTL {
			execStore.Delete(k)
		}
		return true
	})
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
