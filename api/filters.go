package api

import (
	"encoding/json"
	"strings"

	"github.com/lunguini/gocker/engine"
)

// listFilters models the Docker API filter query param for /containers/json
// (and similar list endpoints). The wire format is a JSON object mapping a
// filter category (label, name, id, status, …) to either a list of match
// strings or a map[string]bool "set". Compose v2 uses the map form:
//
//   {"label": {"com.docker.compose.project=tmf": true,
//              "com.docker.compose.service=server": true,
//              "com.docker.compose.config-hash": true}}
//
// Each label entry is either "key=value" (exact match) or a bare "key"
// (presence check).
type listFilters struct {
	labels []labelConstraint
	names  []string
	ids    []string
}

type labelConstraint struct {
	key   string
	value string
	hasV  bool
}

func (f *listFilters) match(c engine.ContainerInfo) bool {
	for _, cst := range f.labels {
		v, ok := c.Labels[cst.key]
		if !ok {
			return false
		}
		if cst.hasV && v != cst.value {
			return false
		}
	}
	for _, want := range f.names {
		found := false
		if strings.TrimPrefix(c.Name, "/") == strings.TrimPrefix(want, "/") {
			found = true
		}
		if !found {
			return false
		}
	}
	for _, want := range f.ids {
		if c.ID != want && !strings.HasPrefix(c.ID, want) {
			return false
		}
	}
	return true
}

// parseListFilters decodes Docker's filters param. Accepts both the map
// shape ({"label":{"k":true}}) compose v2 uses and the array shape
// ({"label":["k=v"]}) older clients send.
func parseListFilters(raw string) (listFilters, error) {
	var f listFilters
	if raw == "" {
		return f, nil
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return f, err
	}
	for key, body := range decoded {
		entries, err := decodeFilterEntries(body)
		if err != nil {
			return f, err
		}
		switch key {
		case "label":
			for _, e := range entries {
				f.labels = append(f.labels, parseLabelConstraint(e))
			}
		case "name":
			f.names = append(f.names, entries...)
		case "id":
			f.ids = append(f.ids, entries...)
		}
		// Unknown filter categories are silently ignored — the spec allows
		// unknown filters to pass through and we don't want compose to
		// fail on a category we haven't implemented yet.
	}
	return f, nil
}

func decodeFilterEntries(body json.RawMessage) ([]string, error) {
	var asMap map[string]bool
	if err := json.Unmarshal(body, &asMap); err == nil {
		out := make([]string, 0, len(asMap))
		for k, v := range asMap {
			if v {
				out = append(out, k)
			}
		}
		return out, nil
	}
	var asList []string
	if err := json.Unmarshal(body, &asList); err == nil {
		return asList, nil
	}
	// Docker also accepts a single string for some categories.
	var asStr string
	if err := json.Unmarshal(body, &asStr); err == nil && asStr != "" {
		return []string{asStr}, nil
	}
	return nil, nil
}

func parseLabelConstraint(entry string) labelConstraint {
	if i := strings.Index(entry, "="); i >= 0 {
		return labelConstraint{key: entry[:i], value: entry[i+1:], hasV: true}
	}
	return labelConstraint{key: entry}
}
