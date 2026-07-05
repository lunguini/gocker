package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/lunguini/gocker/engine"
)

type Server struct {
	eng        engine.Runtime
	socketPath string
	mux        *http.ServeMux
	events     *EventBus
	logger     *Logger
	version    string
}

// NewServer builds an API server. An optional version string (from ldflags)
// is threaded into the /version and /info responses; when omitted it falls
// back to "dev".
func NewServer(eng engine.Runtime, socketPath string, version ...string) *Server {
	ver := "dev"
	if len(version) > 0 && version[0] != "" {
		ver = version[0]
	}
	s := &Server{
		eng:        eng,
		socketPath: socketPath,
		mux:        http.NewServeMux(),
		events:     NewEventBus(),
		version:    ver,
	}
	s.registerRoutes()
	return s
}

// versionPrefix matches Docker's API version segment (e.g. "/v1.41/"). Only a
// well-formed major.minor prefix is stripped; unversioned paths like
// "/volumes/myvol" (which start with 'v' but aren't a version) are left alone.
// Group 2 is the remainder of the path (starting with '/'), or empty.
var versionPrefix = regexp.MustCompile(`^(/v\d+\.\d+)(/.*)?$`)

func (s *Server) registerRoutes() {
	// System
	s.mux.HandleFunc("GET /_ping", s.handlePing)
	s.mux.HandleFunc("HEAD /_ping", s.handlePing)
	s.mux.HandleFunc("GET /version", s.handleVersion)
	s.mux.HandleFunc("GET /info", s.handleInfo)
	s.mux.HandleFunc("GET /system/df", s.handleSystemDf)

	// Events
	s.mux.HandleFunc("GET /events", s.handleEvents)

	// Containers
	s.mux.HandleFunc("GET /containers/json", s.handleContainerList)
	s.mux.HandleFunc("POST /containers/create", s.handleContainerCreate)
	s.mux.HandleFunc("POST /containers/{id}/start", s.handleContainerStart)
	s.mux.HandleFunc("POST /containers/{id}/stop", s.handleContainerStop)
	s.mux.HandleFunc("POST /containers/{id}/kill", s.handleContainerKill)
	s.mux.HandleFunc("DELETE /containers/{id}", s.handleContainerRemove)
	s.mux.HandleFunc("GET /containers/{id}/json", s.handleContainerInspect)
	s.mux.HandleFunc("GET /containers/{id}/logs", s.handleContainerLogs)
	s.mux.HandleFunc("GET /containers/{id}/stats", s.handleContainerStats)
	s.mux.HandleFunc("POST /containers/{id}/exec", s.handleExecCreate)
	s.mux.HandleFunc("POST /exec/{id}/start", s.handleExecStart)
	s.mux.HandleFunc("GET /exec/{id}/json", s.handleExecInspect)

	// Images
	s.mux.HandleFunc("GET /images/json", s.handleImageList)
	s.mux.HandleFunc("POST /images/create", s.handleImagePull)
	s.mux.HandleFunc("DELETE /images/{name...}", s.handleImageRemove)
	s.mux.HandleFunc("GET /images/{name...}", s.handleImageInspect)

	// Networks
	s.mux.HandleFunc("GET /networks", s.handleNetworkList)
	s.mux.HandleFunc("GET /networks/{id}", s.handleNetworkInspect)
	s.mux.HandleFunc("POST /networks/create", s.handleNetworkCreate)
	s.mux.HandleFunc("DELETE /networks/{id}", s.handleNetworkRemove)
	s.mux.HandleFunc("POST /networks/{id}/connect", s.handleNetworkConnect)
	s.mux.HandleFunc("POST /networks/{id}/disconnect", s.handleNetworkDisconnect)

	// Volumes
	s.mux.HandleFunc("GET /volumes", s.handleVolumeList)
	s.mux.HandleFunc("POST /volumes/create", s.handleVolumeCreate)
	s.mux.HandleFunc("DELETE /volumes/{name}", s.handleVolumeRemove)
	s.mux.HandleFunc("GET /volumes/{name}", s.handleVolumeInspect)
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	_ = os.Remove(s.socketPath)

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer func() { _ = listener.Close() }()

	_ = os.Chmod(s.socketPath, 0660)

	var handler http.Handler = s
	if s.logger != nil {
		handler = loggingMiddleware(s, s.logger)
	}
	srv := &http.Server{Handler: handler}

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ServeHTTP strips the API version prefix (e.g., /v1.41/) and delegates to the
// mux. Only a real /vMAJOR.MINOR/ prefix is stripped — unversioned routes such
// as /volumes/myvol or /networks/... start with 'v' but must pass through
// untouched (the old naive stripper turned /volumes/myvol into /myvol → 404).
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m := versionPrefix.FindStringSubmatch(r.URL.Path); m != nil {
		stripped := m[2]
		if stripped == "" {
			stripped = "/"
		}
		r.URL.Path = stripped
		r.RequestURI = stripped
		if r.URL.RawQuery != "" {
			r.RequestURI = stripped + "?" + r.URL.RawQuery
		}
	}
	s.mux.ServeHTTP(w, r)
}
