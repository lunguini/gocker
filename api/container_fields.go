package api

import (
	"strconv"
	"strings"
)

// deriveContainerState normalizes a Runtime-reported State/Status pair into
// one of the Docker API state strings (running, exited, created, paused,
// restarting, removing, dead, "") that lazydocker and the Docker Go SDK
// key rendering and filtering off. nerdctl's `ps --format json` only emits
// Status ("Up 2 minutes", "Exited (0) ..."), and the Apple CLI's upstream
// JSON uses its own status keys — both leave State empty by default, and
// an empty State makes clients treat the container as unknown.
func deriveContainerState(state, status string) string {
	if s := strings.ToLower(strings.TrimSpace(state)); s != "" {
		switch s {
		case "running", "exited", "created", "paused", "restarting", "removing", "dead":
			return s
		}
	}
	s := strings.ToLower(strings.TrimSpace(status))
	switch {
	case s == "":
		return ""
	case strings.HasPrefix(s, "up"):
		return "running"
	case strings.HasPrefix(s, "exited"):
		return "exited"
	case strings.HasPrefix(s, "created"):
		return "created"
	case strings.HasPrefix(s, "paused"):
		return "paused"
	case strings.HasPrefix(s, "restarting"):
		return "restarting"
	case strings.HasPrefix(s, "removing"):
		return "removing"
	case strings.HasPrefix(s, "dead"):
		return "dead"
	case strings.HasPrefix(s, "stopped"):
		return "exited"
	}
	return ""
}

// parseNerdctlPorts converts nerdctl's flat ports string ("0.0.0.0:8080->80/tcp,
// [::]:8080->80/tcp") into the []PortMapping shape Docker API clients expect.
// Returns an empty (non-nil) slice when input is blank so the JSON marshals as
// [] instead of null — some clients are strict.
func parseNerdctlPorts(raw string) []PortMapping {
	out := []PortMapping{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// Split container-side "PROTO" suffix first ("80/tcp").
		proto := "tcp"
		if i := strings.LastIndex(entry, "/"); i != -1 {
			proto = entry[i+1:]
			entry = entry[:i]
		}
		// Split published side ("host:port->cport") from container-only ("cport").
		var publicStr, privateStr, ip string
		if arrow := strings.Index(entry, "->"); arrow != -1 {
			pub := entry[:arrow]
			privateStr = entry[arrow+2:]
			if i := strings.LastIndex(pub, ":"); i != -1 {
				ip, publicStr = pub[:i], pub[i+1:]
			} else {
				publicStr = pub
			}
		} else {
			privateStr = entry
		}
		privPort, _ := parsePort(privateStr)
		pubPort, _ := parsePort(publicStr)
		out = append(out, PortMapping{
			IP:          ip,
			PrivatePort: privPort,
			PublicPort:  pubPort,
			Type:        proto,
		})
	}
	return out
}

func parsePort(s string) (uint16, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(n), nil
}
