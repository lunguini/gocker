package jsonx

import "testing"

func TestGetString(t *testing.T) {
	m := map[string]any{
		"ID":   "abc123",
		"name": "test-container",
	}

	t.Run("first key matches", func(t *testing.T) {
		result := GetString(m, "ID")
		if result != "abc123" {
			t.Errorf("GetString(m, \"ID\") = %q, want %q", result, "abc123")
		}
	})

	t.Run("second key matches", func(t *testing.T) {
		result := GetString(m, "id", "ID")
		if result != "abc123" {
			t.Errorf("GetString(m, \"id\", \"ID\") = %q, want %q", result, "abc123")
		}
	})

	t.Run("case sensitive lookup", func(t *testing.T) {
		result := GetString(m, "name")
		if result != "test-container" {
			t.Errorf("GetString(m, \"name\") = %q, want %q", result, "test-container")
		}
	})

	t.Run("missing key returns empty", func(t *testing.T) {
		result := GetString(m, "nonexistent", "alsoMissing")
		if result != "" {
			t.Errorf("GetString(m, \"nonexistent\") = %q, want empty string", result)
		}
	})

	t.Run("nil value is skipped, falls through to next key", func(t *testing.T) {
		withNull := map[string]any{"id": nil, "Id": "abc123"}
		result := GetString(withNull, "id", "Id")
		if result != "abc123" {
			t.Errorf("GetString(withNull, \"id\", \"Id\") = %q, want %q (nil should not shadow a later key)", result, "abc123")
		}
	})

	t.Run("nil is the only candidate returns empty, not the literal <nil>", func(t *testing.T) {
		withNull := map[string]any{"id": nil}
		result := GetString(withNull, "id")
		if result != "" {
			t.Errorf("GetString(withNull, \"id\") = %q, want empty string", result)
		}
	})

	t.Run("integral float formats without exponent or decimal noise", func(t *testing.T) {
		withNum := map[string]any{"count": float64(12)}
		result := GetString(withNum, "count")
		if result != "12" {
			t.Errorf("GetString(withNum, \"count\") = %q, want %q", result, "12")
		}
	})

	t.Run("fractional float keeps its decimal", func(t *testing.T) {
		withNum := map[string]any{"ratio": 1.5}
		result := GetString(withNum, "ratio")
		if result != "1.5" {
			t.Errorf("GetString(withNum, \"ratio\") = %q, want %q", result, "1.5")
		}
	})
}

func TestExtractStringMap(t *testing.T) {
	t.Run("matches first present key", func(t *testing.T) {
		m := map[string]any{"Labels": map[string]any{"a": "1", "b": 2}}
		got := ExtractStringMap(m, "labels", "Labels")
		if got["a"] != "1" || got["b"] != "2" {
			t.Errorf("ExtractStringMap = %v, want a=1 b=2", got)
		}
	})

	t.Run("no match returns empty non-nil map", func(t *testing.T) {
		got := ExtractStringMap(map[string]any{}, "labels", "Labels")
		if got == nil || len(got) != 0 {
			t.Errorf("ExtractStringMap = %v, want empty non-nil map", got)
		}
	})
}

func TestExtractLabels(t *testing.T) {
	t.Run("top-level labels win", func(t *testing.T) {
		m := map[string]any{"labels": map[string]any{"k": "v"}}
		got := ExtractLabels(m)
		if got["k"] != "v" {
			t.Errorf("ExtractLabels = %v, want k=v", got)
		}
	})

	t.Run("falls back to nested config.labels", func(t *testing.T) {
		m := map[string]any{"config": map[string]any{"labels": map[string]any{"k": "v"}}}
		got := ExtractLabels(m)
		if got["k"] != "v" {
			t.Errorf("ExtractLabels = %v, want k=v from nested config", got)
		}
	})

	t.Run("no labels anywhere returns empty non-nil map", func(t *testing.T) {
		got := ExtractLabels(map[string]any{})
		if got == nil || len(got) != 0 {
			t.Errorf("ExtractLabels = %v, want empty non-nil map", got)
		}
	})
}

func TestExtractLabelsFromAny(t *testing.T) {
	t.Run("only string values are kept", func(t *testing.T) {
		m := map[string]any{"labels": map[string]any{"a": "1", "b": 2}}
		got := ExtractLabelsFromAny(m)
		if got["a"] != "1" {
			t.Errorf("ExtractLabelsFromAny[a] = %q, want %q", got["a"], "1")
		}
		if _, ok := got["b"]; ok {
			t.Errorf("ExtractLabelsFromAny should have dropped non-string value for %q, got %v", "b", got)
		}
	})

	t.Run("falls back to nested config.labels", func(t *testing.T) {
		m := map[string]any{"Config": map[string]any{"Labels": map[string]any{"k": "v"}}}
		got := ExtractLabelsFromAny(m)
		if got["k"] != "v" {
			t.Errorf("ExtractLabelsFromAny = %v, want k=v from nested Config", got)
		}
	})

	t.Run("no labels anywhere returns empty non-nil map", func(t *testing.T) {
		got := ExtractLabelsFromAny(map[string]any{})
		if got == nil || len(got) != 0 {
			t.Errorf("ExtractLabelsFromAny = %v, want empty non-nil map", got)
		}
	})
}

func TestInspectStatus(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"single object flat status", `{"status":"running"}`, "running"},
		{"array of objects flat status", `[{"status":"running","configuration":{"id":"x"}}]`, "running"},
		{"nested State.Status", `{"State":{"Status":"exited"}}`, "exited"},
		{"nested lowercase state.status", `{"state":{"status":"exited"}}`, "exited"},
		{"flat capitalized Status", `{"Status":"stopped"}`, "stopped"},
		{"malformed JSON falls back to string scan", `not json but has "status":"scanned" in it`, "scanned"},
		{"malformed JSON with capitalized Status scan", `garbage "Status":"scanned2" trailer`, "scanned2"},
		{"nothing found returns empty", `{"configuration":{"id":"x"}}`, ""},
		{"empty array returns empty", `[]`, ""},
		{"totally unparseable returns empty", `not json at all`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := InspectStatus([]byte(c.raw))
			if got != c.want {
				t.Errorf("InspectStatus(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}
