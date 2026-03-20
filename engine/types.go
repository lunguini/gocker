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
}

type VolumeInfo struct {
	Name       string
	Driver     string
	Mountpoint string
	Created    time.Time
}

type InspectData struct {
	Raw []byte // raw JSON from `container inspect`
}
