package engine

import "time"

type ContainerInfo struct {
	ID      string
	Name    string
	Image   string
	State   string
	Status  string
	IP      string
	Ports   string
	Created time.Time
	Command string
	// Labels set at create time. Compose reads
	// com.docker.compose.{project,service,oneoff,config-hash} off list
	// responses to scope `docker compose ps` to its own project and fill
	// the Service column. Missing labels → blank service name and compose
	// treats the container as foreign.
	Labels map[string]string
}

type ImageInfo struct {
	ID      string
	Name    string
	Tag     string
	Digest  string
	Size    string
	Created time.Time
	Arch    string
}

type NetworkInfo struct {
	ID     string
	Name   string
	Driver string
	Scope  string
	// Labels are the resource labels set at create time. Critical for
	// Docker Compose compatibility: compose reads
	// com.docker.compose.project to decide whether a network is "its own"
	// vs foreign. Returning empty labels causes compose to refuse its own
	// networks with "not created by Docker Compose".
	Labels map[string]string
}

type VolumeInfo struct {
	Name       string
	Driver     string
	Mountpoint string
	Created    time.Time
	// Labels — see NetworkInfo.Labels. Compose checks
	// com.docker.compose.project here too.
	Labels map[string]string
}

type InspectData struct {
	Raw []byte // raw JSON from `container inspect`
}
