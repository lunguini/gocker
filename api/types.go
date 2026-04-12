package api

// Docker API compatible response types

type ContainerJSON struct {
	ID              string            `json:"Id"`
	Names           []string          `json:"Names,omitempty"`
	Image           string            `json:"Image"`
	ImageID         string            `json:"ImageID,omitempty"`
	Command         string            `json:"Command"`
	Created         int64             `json:"Created"`
	State           any               `json:"State"` // string for list, object for inspect
	Status          string            `json:"Status"`
	Ports           []PortMapping     `json:"Ports"`
	Labels          map[string]string `json:"Labels,omitempty"`
	NetworkSettings *NetworkSettings  `json:"NetworkSettings,omitempty"`
	Config          *ContainerConfig  `json:"Config,omitempty"`
	HostConfig      *HostConfig       `json:"HostConfig,omitempty"`
}

type ContainerState struct {
	Status     string `json:"Status"`
	Running    bool   `json:"Running"`
	Paused     bool   `json:"Paused"`
	StartedAt  string `json:"StartedAt"`
	FinishedAt string `json:"FinishedAt"`
}

type PortMapping struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort,omitempty"`
	Type        string `json:"Type"`
}

type NetworkSettings struct {
	Networks map[string]*EndpointSettings `json:"Networks,omitempty"`
}

type EndpointSettings struct {
	IPAddress string `json:"IPAddress"`
	Gateway   string `json:"Gateway"`
}

type ContainerConfig struct {
	Image string            `json:"Image"`
	Cmd   []string          `json:"Cmd,omitempty"`
	Env   []string          `json:"Env,omitempty"`
	Tty   bool              `json:"Tty"`
	Labels map[string]string `json:"Labels,omitempty"`
}

type HostConfig struct {
	Binds        []string               `json:"Binds,omitempty"`
	PortBindings map[string][]PortBind  `json:"PortBindings,omitempty"`
	NetworkMode  string                 `json:"NetworkMode"`
}

type PortBind struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type CreateContainerRequest struct {
	Image      string            `json:"Image"`
	Cmd        []string          `json:"Cmd,omitempty"`
	Env        []string          `json:"Env,omitempty"`
	Tty        bool              `json:"Tty"`
	OpenStdin  bool              `json:"OpenStdin"`
	WorkingDir string            `json:"WorkingDir,omitempty"`
	Labels     map[string]string `json:"Labels,omitempty"`
	HostConfig *HostConfig       `json:"HostConfig,omitempty"`
}

type CreateContainerResponse struct {
	ID       string   `json:"Id"`
	Warnings []string `json:"Warnings"`
}

type VersionResponse struct {
	Version    string `json:"Version"`
	APIVersion string `json:"ApiVersion"`
	OS         string `json:"Os"`
	Arch       string `json:"Arch"`
	GoVersion  string `json:"GoVersion"`
}

type InfoResponse struct {
	Containers int    `json:"Containers"`
	Images     int    `json:"Images"`
	OSType     string `json:"OSType"`
	Arch       string `json:"Architecture"`
	Name       string `json:"Name"`
	ServerVersion string `json:"ServerVersion"`
}

type ImageJSON struct {
	ID       string   `json:"Id"`
	RepoTags []string `json:"RepoTags"`
	Created  int64    `json:"Created"`
	Size     int64    `json:"Size"`
}

type NetworkJSON struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Driver string `json:"Driver"`
	Scope  string `json:"Scope"`
}

type NetworkCreateRequest struct {
	Name   string `json:"Name"`
	Driver string `json:"Driver,omitempty"`
}

type NetworkConnectRequest struct {
	Container string `json:"Container"`
}

type VolumeJSON struct {
	Name       string `json:"Name"`
	Driver     string `json:"Driver"`
	Mountpoint string `json:"Mountpoint"`
}

type VolumeListResponse struct {
	Volumes []*VolumeJSON `json:"Volumes"`
}

type VolumeCreateRequest struct {
	Name   string `json:"Name"`
	Driver string `json:"Driver,omitempty"`
}

type ExecConfig struct {
	AttachStdin  bool     `json:"AttachStdin"`
	AttachStdout bool     `json:"AttachStdout"`
	AttachStderr bool     `json:"AttachStderr"`
	Tty          bool     `json:"Tty"`
	Cmd          []string `json:"Cmd"`
}

type ExecCreateResponse struct {
	ID string `json:"Id"`
}

type ExecStartRequest struct {
	Detach bool `json:"Detach"`
	Tty    bool `json:"Tty"`
}

// Docker-compatible event types for GET /events streaming endpoint.

type Event struct {
	Type     string     `json:"Type"`
	Action   string     `json:"Action"`
	Actor    EventActor `json:"Actor"`
	Time     int64      `json:"time"`
	TimeNano int64      `json:"timeNano"`
	Scope    string     `json:"scope"`
}

type EventActor struct {
	ID         string            `json:"ID"`
	Attributes map[string]string `json:"Attributes"`
}
