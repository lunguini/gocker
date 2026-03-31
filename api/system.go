package api

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
)

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("API-Version", "1.41")
	_, _ = w.Write([]byte("OK"))
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	resp := VersionResponse{
		Version:    "gocker-0.1.0",
		APIVersion: "1.41",
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		GoVersion:  runtime.Version(),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	containers, _ := s.eng.ContainerList(ctx, true)
	images, _ := s.eng.ImageList(ctx)

	hostname, _ := os.Hostname()
	resp := InfoResponse{
		Containers:    len(containers),
		Images:        len(images),
		OSType:        runtime.GOOS,
		Arch:          runtime.GOARCH,
		Name:          hostname,
		ServerVersion: "gocker-0.1.0",
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"message": message})
}
