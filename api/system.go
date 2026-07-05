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
		Version:    s.version,
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
		ServerVersion: s.version,
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleSystemDf aggregates image/container/volume summaries into the shape
// Docker's `docker system df` (and `types.DiskUsage` SDK type) expects. Build
// cache and layer bytes are stubbed — gocker doesn't track those today.
func (s *Server) handleSystemDf(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	images, _ := s.eng.ImageList(ctx)
	imgSummaries := make([]map[string]any, 0, len(images))
	for _, img := range images {
		repoTag := img.Name
		if img.Tag != "" {
			repoTag = img.Name + ":" + img.Tag
		}
		imgSummaries = append(imgSummaries, map[string]any{
			"Id":          img.ID,
			"ParentId":    "",
			"RepoTags":    []string{repoTag},
			"RepoDigests": []string{},
			"Created":     img.Created.Unix(),
			"Size":        img.SizeBytes,
			"SharedSize":  0,
			"VirtualSize": img.SizeBytes,
			"Labels":      map[string]string{},
			"Containers":  int64(-1),
		})
	}

	containers, _ := s.eng.ContainerList(ctx, true)
	ctrSummaries := make([]map[string]any, 0, len(containers))
	for _, c := range containers {
		ctrSummaries = append(ctrSummaries, map[string]any{
			"Id":         c.ID,
			"Names":      []string{"/" + c.Name},
			"Image":      c.Image,
			"ImageID":    "",
			"Command":    c.Command,
			"Created":    c.Created.Unix(),
			"State":      c.State,
			"Status":     c.Status,
			"Ports":      []any{},
			"Labels":     map[string]string{},
			"SizeRw":     0,
			"SizeRootFs": 0,
		})
	}

	volumes, _ := s.eng.VolumeList(ctx)
	volSummaries := make([]map[string]any, 0, len(volumes))
	for _, v := range volumes {
		volSummaries = append(volSummaries, map[string]any{
			"Name":       v.Name,
			"Driver":     v.Driver,
			"Mountpoint": v.Mountpoint,
			"Scope":      "local",
			"Labels":     map[string]string{},
			"Options":    map[string]string{},
			"UsageData": map[string]int64{
				"Size":     0,
				"RefCount": -1,
			},
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"LayersSize": 0,
		"Images":     imgSummaries,
		"Containers": ctrSummaries,
		"Volumes":    volSummaries,
		"BuildCache": []any{},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"message": message})
}
