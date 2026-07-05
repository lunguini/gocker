package api

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockernetwork "github.com/docker/docker/api/types/network"
	dockervolume "github.com/docker/docker/api/types/volume"
	"github.com/docker/go-connections/nat"

	"github.com/lunguini/gocker/internal/jsonx"
)

// initNilCollections walks v via reflection and initializes every nil slice
// and map to an empty value of the correct type. Leaves nil pointers alone
// (pointer-to-primitive fields on Docker types are idiomatic "unset"
// sentinels — e.g. Resources.MemorySwappiness=nil means "no limit", not
// "limit zero"). Struct-pointer fields that clients deref without a nil-
// check are still explicitly initialized in the dedicated helpers below.
//
// This is the key "don't whack-a-mole" mechanism: any nil slice/map on any
// Docker SDK type decoded from our inspect response gets filled in
// automatically, without us having to enumerate every field.
func initNilCollections(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			initNilCollections(v.Elem())
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !v.Type().Field(i).IsExported() {
				continue
			}
			initNilCollections(v.Field(i))
		}
	case reflect.Slice:
		if v.IsNil() && v.CanSet() {
			v.Set(reflect.MakeSlice(v.Type(), 0, 0))
		} else {
			for i := 0; i < v.Len(); i++ {
				initNilCollections(v.Index(i))
			}
		}
	case reflect.Map:
		if v.IsNil() && v.CanSet() {
			v.Set(reflect.MakeMap(v.Type()))
		} else {
			iter := v.MapRange()
			for iter.Next() {
				initNilCollections(iter.Value())
			}
		}
	}
}

// This file is the single source of truth for "what an inspect response
// looks like on the wire". Instead of passing raw backend JSON through with
// hand-applied patches, we decode into the real Docker SDK types, guarantee
// every pointer/map/slice field is non-nil, then marshal back.
//
// Benefits over the previous ad-hoc patching:
//   - A field that's safe to deref for one client is safe for all of them —
//     we can't "miss" a field because we didn't realize a client used it.
//   - Every field we serialize matches what dockerd emits, so any Docker
//     SDK client decodes without surprises.
//   - Adding a new client/tool to our support list is a no-op: if the SDK
//     exposes the field, we already emit it.

// reshapeContainerInspect takes a raw inspect payload (from Apple CLI or
// nerdctl) and returns a fully-populated types.ContainerJSON that's safe
// to marshal and return to any Docker SDK client. Ensures every pointer
// field inside ContainerJSON and its nested structs is non-nil.
func reshapeContainerInspect(raw []byte) (dockertypes.ContainerJSON, error) {
	var c dockertypes.ContainerJSON

	// Apple CLI returns a JSON array; nerdctl returns an array too (most
	// versions) or a single object. Try both.
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var arr []dockertypes.ContainerJSON
		if err := json.Unmarshal(raw, &arr); err != nil {
			return c, fmt.Errorf("decode container inspect array: %w", err)
		}
		if len(arr) == 0 {
			return c, fmt.Errorf("inspect returned empty array")
		}
		c = arr[0]
	} else {
		if err := json.Unmarshal(raw, &c); err != nil {
			return c, fmt.Errorf("decode container inspect: %w", err)
		}
	}

	ensureNonNilContainerJSON(&c)
	// Apple CLI's flat shape emits a top-level "status" string with no
	// nested State object. The SDK decode above left State empty; patch
	// in what we can from the raw map.
	if c.State != nil && c.State.Status == "" {
		if m := rawAsMap(raw); m != nil {
			if s := jsonx.GetString(m, "status", "Status"); s != "" {
				c.State.Status = s
				c.State.Running = strings.EqualFold(s, "running")
			}
		}
	}
	return c, nil
}

// rawAsMap decodes raw JSON into a map, handling single-object and array-
// of-one-object shapes. Returns nil on decode failure.
func rawAsMap(raw []byte) map[string]any {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var arr []map[string]any
		if err := json.Unmarshal(raw, &arr); err != nil || len(arr) == 0 {
			return nil
		}
		return arr[0]
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// ensureNonNilContainerJSON fills every pointer and slice/map field in the
// ContainerJSON tree with a zero-value allocation when it's nil. Clients
// that call e.g. `result.Config.Tty`, `result.State.Running`,
// `result.HostConfig.NetworkMode`, `result.NetworkSettings.Networks` all
// work without nil-checks.
func ensureNonNilContainerJSON(c *dockertypes.ContainerJSON) {
	if c.ContainerJSONBase == nil {
		c.ContainerJSONBase = &dockertypes.ContainerJSONBase{}
	}
	base := c.ContainerJSONBase
	if base.State == nil {
		base.State = &dockertypes.ContainerState{}
	}
	// Zero-valued State is still safe to deref even if Status/Running aren't
	// populated — the primary goal here is "no nil pointers", not accurate
	// lifecycle reporting for Apple CLI's flat shape.
	if base.HostConfig == nil {
		base.HostConfig = &dockercontainer.HostConfig{}
	}
	ensureNonNilHostConfig(base.HostConfig)
	if base.Args == nil {
		base.Args = []string{}
	}
	if base.ExecIDs == nil {
		base.ExecIDs = []string{}
	}

	if c.Config == nil {
		c.Config = &dockercontainer.Config{}
	}
	ensureNonNilContainerConfig(c.Config)

	if c.NetworkSettings == nil {
		c.NetworkSettings = &dockertypes.NetworkSettings{}
	}
	ensureNonNilNetworkSettings(c.NetworkSettings)

	if c.Mounts == nil {
		c.Mounts = []dockertypes.MountPoint{}
	}

	// Apple/nerdctl sometimes emit Name without the leading slash Docker
	// clients expect.
	if base.Name != "" && !strings.HasPrefix(base.Name, "/") {
		base.Name = "/" + base.Name
	}

	// Sweep the whole tree to auto-init any remaining nil slices/maps.
	// Catches fields we haven't named explicitly — the whole point of
	// the refactor is to make new clients/fields safe without patching.
	initNilCollections(reflect.ValueOf(c).Elem())
}

func ensureNonNilContainerConfig(cfg *dockercontainer.Config) {
	if cfg.Env == nil {
		cfg.Env = []string{}
	}
	if cfg.Cmd == nil {
		cfg.Cmd = []string{}
	}
	if cfg.Entrypoint == nil {
		cfg.Entrypoint = []string{}
	}
	if cfg.OnBuild == nil {
		cfg.OnBuild = []string{}
	}
	if cfg.Shell == nil {
		cfg.Shell = []string{}
	}
	if cfg.Labels == nil {
		cfg.Labels = map[string]string{}
	}
	if cfg.Volumes == nil {
		cfg.Volumes = map[string]struct{}{}
	}
	if cfg.ExposedPorts == nil {
		cfg.ExposedPorts = nat.PortSet{}
	}
}

func ensureNonNilHostConfig(hc *dockercontainer.HostConfig) {
	if hc.Binds == nil {
		hc.Binds = []string{}
	}
	if hc.Links == nil {
		hc.Links = []string{}
	}
	if hc.VolumesFrom == nil {
		hc.VolumesFrom = []string{}
	}
	if hc.CapAdd == nil {
		hc.CapAdd = []string{}
	}
	if hc.CapDrop == nil {
		hc.CapDrop = []string{}
	}
	if hc.DNS == nil {
		hc.DNS = []string{}
	}
	if hc.DNSOptions == nil {
		hc.DNSOptions = []string{}
	}
	if hc.DNSSearch == nil {
		hc.DNSSearch = []string{}
	}
	if hc.ExtraHosts == nil {
		hc.ExtraHosts = []string{}
	}
	if hc.GroupAdd == nil {
		hc.GroupAdd = []string{}
	}
	if hc.SecurityOpt == nil {
		hc.SecurityOpt = []string{}
	}
	if hc.PortBindings == nil {
		hc.PortBindings = nat.PortMap{}
	}
	if hc.LogConfig.Type == "" {
		hc.LogConfig.Type = "json-file"
	}
	if hc.LogConfig.Config == nil {
		hc.LogConfig.Config = map[string]string{}
	}
	if hc.RestartPolicy.Name == "" {
		hc.RestartPolicy.Name = "no"
	}
	if hc.NetworkMode == "" {
		hc.NetworkMode = "default"
	}
	// Resources.* and other deeply-nested slice fields are handled by the
	// reflection sweep in initNilCollections — no need to enumerate each.
}

func ensureNonNilNetworkSettings(ns *dockertypes.NetworkSettings) {
	if ns.Networks == nil {
		ns.Networks = map[string]*dockernetwork.EndpointSettings{}
	}
	if ns.Ports == nil {
		ns.Ports = nat.PortMap{}
	}
	if ns.SecondaryIPAddresses == nil {
		ns.SecondaryIPAddresses = []dockernetwork.Address{}
	}
	if ns.SecondaryIPv6Addresses == nil {
		ns.SecondaryIPv6Addresses = []dockernetwork.Address{}
	}
}

// reshapeNetworkInspect decodes a raw network inspect payload into the real
// SDK type with every map/slice/pointer non-nil. Uses a map-based fill pass
// to pick up fields the SDK decode misses — e.g. Apple Container CLI puts
// labels under config.labels, not top-level.
func reshapeNetworkInspect(raw []byte, requestedID string) (dockertypes.NetworkResource, error) {
	var n dockertypes.NetworkResource
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var arr []dockertypes.NetworkResource
		if err := json.Unmarshal(raw, &arr); err != nil {
			return buildNetworkFromApple(raw, requestedID)
		}
		if len(arr) == 0 {
			return n, fmt.Errorf("inspect returned empty array")
		}
		n = arr[0]
	} else {
		if err := json.Unmarshal(raw, &n); err != nil {
			return buildNetworkFromApple(raw, requestedID)
		}
	}
	// Fill gaps from the raw map — covers Apple's nested config.labels and
	// any field the SDK's lowercase-vs-capitalized tag-matching missed.
	if m := rawAsMap(raw); m != nil {
		if len(n.Labels) == 0 {
			n.Labels = jsonx.ExtractLabels(m)
		}
		if n.ID == "" {
			n.ID = jsonx.GetString(m, "id", "ID", "Id")
		}
		if n.Name == "" {
			n.Name = jsonx.GetString(m, "name", "Name", "id", "ID", "Id")
		}
		if n.Driver == "" {
			n.Driver = jsonx.GetString(m, "driver", "Driver")
		}
	}
	ensureNonNilNetworkResource(&n)
	if n.Name == "" {
		n.Name = requestedID
	}
	return n, nil
}

// buildNetworkFromApple handles Apple Container CLI's network inspect
// output, which has a different top-level shape (fields under `config`).
// Best-effort: populate what we can, leave defaults for the rest.
func buildNetworkFromApple(raw []byte, requestedID string) (dockertypes.NetworkResource, error) {
	var apple []map[string]any
	if err := json.Unmarshal(raw, &apple); err != nil {
		var single map[string]any
		if err := json.Unmarshal(raw, &single); err != nil {
			return dockertypes.NetworkResource{}, fmt.Errorf("apple network inspect decode: %w", err)
		}
		apple = []map[string]any{single}
	}
	if len(apple) == 0 {
		return dockertypes.NetworkResource{}, fmt.Errorf("inspect returned empty array")
	}
	m := apple[0]
	n := dockertypes.NetworkResource{
		ID:     jsonx.GetString(m, "id", "ID", "Id"),
		Name:   jsonx.GetString(m, "name", "Name", "id", "ID"),
		Driver: jsonx.GetString(m, "driver", "Driver"),
		Scope:  "local",
		Labels: jsonx.ExtractLabels(m),
	}
	if n.Name == "" {
		n.Name = requestedID
	}
	ensureNonNilNetworkResource(&n)
	return n, nil
}

func ensureNonNilNetworkResource(n *dockertypes.NetworkResource) {
	if n.Containers == nil {
		n.Containers = map[string]dockertypes.EndpointResource{}
	}
	if n.Options == nil {
		n.Options = map[string]string{}
	}
	if n.Labels == nil {
		n.Labels = map[string]string{}
	}
	if n.Peers == nil {
		n.Peers = []dockernetwork.PeerInfo{}
	}
	if n.Services == nil {
		n.Services = map[string]dockernetwork.ServiceInfo{}
	}
	if n.IPAM.Options == nil {
		n.IPAM.Options = map[string]string{}
	}
	if n.IPAM.Config == nil {
		n.IPAM.Config = []dockernetwork.IPAMConfig{}
	}
	if n.IPAM.Driver == "" {
		n.IPAM.Driver = "default"
	}
	if n.Driver == "" {
		n.Driver = "bridge"
	}
	if n.Scope == "" {
		n.Scope = "local"
	}
}

// reshapeVolumeInspect decodes a raw volume inspect payload into the real
// SDK volume.Volume with non-nil maps. Falls back to map-based extraction
// for fields Apple CLI names differently (e.g. `source` → Mountpoint).
func reshapeVolumeInspect(raw []byte, requestedName string) (dockervolume.Volume, error) {
	var v dockervolume.Volume
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var arr []dockervolume.Volume
		if err := json.Unmarshal(raw, &arr); err != nil {
			return buildVolumeFromApple(raw, requestedName)
		}
		if len(arr) == 0 {
			return v, fmt.Errorf("inspect returned empty array")
		}
		v = arr[0]
	} else {
		if err := json.Unmarshal(raw, &v); err != nil {
			return buildVolumeFromApple(raw, requestedName)
		}
	}
	// Fill gaps from the raw map — Apple uses `source` instead of
	// `Mountpoint`, and labels may be case-sensitive differently.
	if m := rawAsMap(raw); m != nil {
		if v.Mountpoint == "" {
			v.Mountpoint = jsonx.GetString(m, "source", "Source", "mountpoint", "Mountpoint")
		}
		if v.Driver == "" {
			v.Driver = jsonx.GetString(m, "driver", "Driver")
		}
		if v.Name == "" {
			v.Name = jsonx.GetString(m, "name", "Name")
		}
		if len(v.Labels) == 0 {
			v.Labels = jsonx.ExtractLabels(m)
		}
	}
	if v.Name == "" {
		v.Name = requestedName
	}
	ensureNonNilVolume(&v)
	return v, nil
}

func buildVolumeFromApple(raw []byte, requestedName string) (dockervolume.Volume, error) {
	var apple []map[string]any
	if err := json.Unmarshal(raw, &apple); err != nil {
		var single map[string]any
		if err := json.Unmarshal(raw, &single); err != nil {
			return dockervolume.Volume{}, fmt.Errorf("apple volume inspect decode: %w", err)
		}
		apple = []map[string]any{single}
	}
	if len(apple) == 0 {
		return dockervolume.Volume{}, fmt.Errorf("inspect returned empty array")
	}
	m := apple[0]
	v := dockervolume.Volume{
		Name:       jsonx.GetString(m, "name", "Name"),
		Driver:     jsonx.GetString(m, "driver", "Driver"),
		Mountpoint: jsonx.GetString(m, "mountpoint", "Mountpoint", "source", "Source"),
		CreatedAt:  jsonx.GetString(m, "createdAt", "CreatedAt", "created", "Created"),
		Scope:      "local",
		Labels:     jsonx.ExtractLabels(m),
	}
	if v.Name == "" {
		v.Name = requestedName
	}
	ensureNonNilVolume(&v)
	return v, nil
}

func ensureNonNilVolume(v *dockervolume.Volume) {
	if v.Labels == nil {
		v.Labels = map[string]string{}
	}
	if v.Options == nil {
		v.Options = map[string]string{}
	}
	if v.Scope == "" {
		v.Scope = "local"
	}
	if v.Driver == "" {
		v.Driver = "local"
	}
}

// Silence unused-import in case some fields disappear from the SDK later.
var _ = time.Now
