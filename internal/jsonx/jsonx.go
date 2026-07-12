// Package jsonx collects small helpers for picking fields out of the
// loosely-typed map[string]any payloads produced by unmarshaling Apple
// Container CLI / nerdctl JSON output. Both backends vary field casing and
// nesting, and json.Unmarshal decodes every JSON number as float64 — these
// helpers absorb that variance in one place instead of drifting across
// per-package copies.
package jsonx

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// GetString looks up the first of keys present in m with a non-null value
// and renders it as a string. A key present with a JSON null value is
// skipped rather than returned as the literal "<nil>" — the next candidate
// key is checked instead. Numbers are formatted without exponent/decimal
// noise (json.Unmarshal decodes all JSON numbers as float64, so an integral
// ID like 12 would otherwise render as "1.2e+01"-style text via %v).
func GetString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		if f, ok := v.(float64); ok {
			if f == math.Trunc(f) && !math.IsInf(f, 0) {
				return strconv.FormatFloat(f, 'f', -1, 64)
			}
			return strconv.FormatFloat(f, 'g', -1, 64)
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// ExtractStringMap pulls a map[string]string out of a raw inspect payload,
// trying each candidate key (for the case-insensitive Apple CLI / nerdctl
// naming split) and returning an empty (non-nil) map if nothing matches.
// Non-string values are rendered via %v so the wire shape still satisfies
// Docker SDK's strict decoders.
func ExtractStringMap(m map[string]any, keys ...string) map[string]string {
	for _, k := range keys {
		raw, ok := m[k]
		if !ok {
			continue
		}
		if mm, ok := raw.(map[string]any); ok {
			out := make(map[string]string, len(mm))
			for k2, v := range mm {
				out[k2] = fmt.Sprintf("%v", v)
			}
			return out
		}
	}
	return map[string]string{}
}

// ExtractLabels is ExtractStringMap pinned to the "labels" / "Labels" keys,
// with a fallback to `config.labels` which is where Apple's `container
// network inspect` nests them. Compose relies on labels being passed
// through verbatim to decide whether a network/volume is "its own" vs
// foreign — returning an empty map causes compose to refuse its own
// resources with "not created by Docker Compose, use external: true".
func ExtractLabels(m map[string]any) map[string]string {
	if labels := ExtractStringMap(m, "labels", "Labels"); len(labels) > 0 {
		return labels
	}
	for _, nestedKey := range []string{"config", "Config"} {
		if nested, ok := m[nestedKey].(map[string]any); ok {
			if labels := ExtractStringMap(nested, "labels", "Labels"); len(labels) > 0 {
				return labels
			}
		}
	}
	return map[string]string{}
}

// ExtractLabelsFromAny pulls a labels map out of a raw JSON object, checking
// the common top-level keys and Apple Container CLI's nested config.labels
// location. Only string-valued entries are kept — unlike ExtractLabels,
// which stringifies everything via %v. Returns a non-nil map so JSON
// marshal emits `{}` instead of `null` — Docker SDK clients sometimes choke
// on null labels.
func ExtractLabelsFromAny(m map[string]any) map[string]string {
	check := func(mp map[string]any, keys ...string) map[string]string {
		for _, k := range keys {
			raw, ok := mp[k]
			if !ok {
				continue
			}
			if lm, ok := raw.(map[string]any); ok && len(lm) > 0 {
				out := make(map[string]string, len(lm))
				for k2, v := range lm {
					if s, ok := v.(string); ok {
						out[k2] = s
					}
				}
				return out
			}
		}
		return nil
	}
	if out := check(m, "labels", "Labels"); out != nil {
		return out
	}
	for _, nestedKey := range []string{"config", "Config"} {
		if nested, ok := m[nestedKey].(map[string]any); ok {
			if out := check(nested, "labels", "Labels"); out != nil {
				return out
			}
		}
	}
	return map[string]string{}
}

// InspectStatus extracts a status string from a raw container/VM inspect
// JSON payload, tolerating the shape variations seen across Apple Container
// CLI and nerdctl output:
//   - a single JSON object, or a JSON array of objects (first element wins)
//   - a nested State.Status (or state.status) string
//   - a flat top-level status or Status string
//   - as a last resort, a raw substring scan for `"status":"..."` /
//     `"Status":"..."` when the structured lookups above find nothing (seen
//     with partial/malformed payloads from in-progress operations)
//
// Returns "" if no status could be found by any of the above. Callers
// decide how to normalize case and what an empty result means (missing
// resource vs. "unknown").
func InspectStatus(raw []byte) string {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		var arr []map[string]any
		if json.Unmarshal(raw, &arr) == nil && len(arr) > 0 {
			obj = arr[0]
		}
	}
	if obj != nil {
		for _, stateKey := range []string{"State", "state"} {
			if state, ok := obj[stateKey].(map[string]any); ok {
				if s := GetString(state, "Status", "status"); s != "" {
					return s
				}
			}
		}
		// Apple container CLI 1.1.0+: "status" is an object holding runtime
		// state — { "status": { "state": "running", "startedDate": "...",
		// "networks": [...] } }. Pre-1.1.0 it was a plain string.
		if status, ok := obj["status"].(map[string]any); ok {
			if s := GetString(status, "state", "State"); s != "" {
				return s
			}
		}
		if s := GetString(obj, "status", "Status"); s != "" {
			return s
		}
	}
	s := string(raw)
	for _, candidate := range []string{`"status":"`, `"Status":"`} {
		if _, after, found := strings.Cut(s, candidate); found {
			if val, _, found := strings.Cut(after, `"`); found {
				return val
			}
		}
	}
	return ""
}
