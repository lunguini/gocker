package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/lunguini/gocker/engine"
)

type Server struct {
	eng        engine.Runtime
	socketPath string
	mux        *http.ServeMux
}

func NewServer(eng engine.Runtime, socketPath string) *Server {
	s := &Server{
		eng:        eng,
		socketPath: socketPath,
		mux:        http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// System
	s.mux.HandleFunc("GET /_ping", s.handlePing)
	s.mux.HandleFunc("HEAD /_ping", s.handlePing)
	s.mux.HandleFunc("GET /version", s.handleVersion)
	s.mux.HandleFunc("GET /info", s.handleInfo)

	// Containers
	s.mux.HandleFunc("GET /containers/json", s.handleContainerList)
	s.mux.HandleFunc("POST /containers/create", s.handleContainerCreate)
	s.mux.HandleFunc("POST /containers/{id}/start", s.handleContainerStart)
	s.mux.HandleFunc("POST /containers/{id}/stop", s.handleContainerStop)
	s.mux.HandleFunc("POST /containers/{id}/kill", s.handleContainerKill)
	s.mux.HandleFunc("DELETE /containers/{id}", s.handleContainerRemove)
	s.mux.HandleFunc("GET /containers/{id}/json", s.handleContainerInspect)
	s.mux.HandleFunc("GET /containers/{id}/logs", s.handleContainerLogs)
	s.mux.HandleFunc("POST /containers/{id}/exec", s.handleExecCreate)
	s.mux.HandleFunc("POST /exec/{id}/start", s.handleExecStart)

	// Images
	s.mux.HandleFunc("GET /images/json", s.handleImageList)
	s.mux.HandleFunc("POST /images/create", s.handleImagePull)
	s.mux.HandleFunc("DELETE /images/{name}", s.handleImageRemove)
	s.mux.HandleFunc("GET /images/{name}/json", s.handleImageInspect)

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
	os.Remove(s.socketPath)

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()

	os.Chmod(s.socketPath, 0660)

	srv := &http.Server{Handler: s}

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ServeHTTP strips the API version prefix (e.g., /v1.41/) and delegates to the mux.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// Strip /v1.XX/ prefix
	if len(path) > 2 && path[0] == '/' && path[1] == 'v' {
		if idx := strings.Index(path[2:], "/"); idx >= 0 {
			stripped := path[2+idx:]
			r.URL.Path = stripped
			r.RequestURI = stripped
			if r.URL.RawQuery != "" {
				r.RequestURI = stripped + "?" + r.URL.RawQuery
			}
		}
	}
	s.mux.ServeHTTP(w, r)
}
