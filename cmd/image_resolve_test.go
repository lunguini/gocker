package cmd

import (
	"context"
	"testing"

	"github.com/lunguini/gocker/engine"
)

type fakeImageLister struct {
	engine.Runtime // embed nil; only ImageList is implemented for these tests
	images         []engine.ImageInfo
}

func (f *fakeImageLister) ImageList(ctx context.Context) ([]engine.ImageInfo, error) {
	return f.images, nil
}

func TestResolveImageReferences(t *testing.T) {
	lister := &fakeImageLister{
		images: []engine.ImageInfo{
			{ID: "sha256:abcdef0123456789aaaa", Name: "redpandadata/redpanda", Tag: "v24.3.1"},
			{ID: "sha256:abcdef0123456789bbbb", Name: "redpandadata/redpanda", Tag: "latest"},
			{ID: "sha256:0000000000000000cccc", Name: "nginx", Tag: "alpine"},
			{ID: "sha256:ffffffffffffffffdddd", Name: "postgres", Tag: "16-alpine"},
		},
	}

	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "explicit tag matches one",
			input: "redpandadata/redpanda:latest",
			want:  []string{"redpandadata/redpanda:latest"},
		},
		{
			name:  "repo-only returns all tags",
			input: "redpandadata/redpanda",
			want:  []string{"redpandadata/redpanda:v24.3.1", "redpandadata/redpanda:latest"},
		},
		{
			name:  "repo-only with a single tag",
			input: "nginx",
			want:  []string{"nginx:alpine"},
		},
		{
			name:  "short ID prefix (12 chars) unique match",
			input: "0000000000000000cccc",
			want:  []string{"nginx:alpine"},
		},
		{
			name:  "unknown name falls through unchanged",
			input: "does/not-exist",
			want:  []string{"does/not-exist"},
		},
		{
			name:  "explicit tag not in list still passes through",
			input: "nginx:custom",
			want:  []string{"nginx:custom"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveImageReferences(context.Background(), lister, tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalStrings(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveImageReferences_AmbiguousIDFails(t *testing.T) {
	lister := &fakeImageLister{
		images: []engine.ImageInfo{
			{ID: "sha256:abcdef1111", Name: "a/img", Tag: "1"},
			{ID: "sha256:abcdef2222", Name: "b/img", Tag: "2"},
		},
	}
	// "abcdef" prefix matches both → ambiguous
	_, err := resolveImageReferences(context.Background(), lister, "abcdef")
	if err == nil {
		t.Fatal("expected ambiguous-ID error")
	}
}

func TestResolveImageReferences_FullSha256IDPrefixMatches(t *testing.T) {
	lister := &fakeImageLister{
		images: []engine.ImageInfo{
			{ID: "sha256:abcdef0123456789aaaa", Name: "nginx", Tag: "alpine"},
		},
	}
	// User pastes the ID with the sha256: prefix — should still match.
	got, err := resolveImageReferences(context.Background(), lister, "sha256:abcdef01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "nginx:alpine" {
		t.Errorf("got %v, want [nginx:alpine]", got)
	}
}

func TestResolveImageReferences_EmptyInputErrors(t *testing.T) {
	lister := &fakeImageLister{}
	if _, err := resolveImageReferences(context.Background(), lister, ""); err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestLooksLikeImageID(t *testing.T) {
	cases := map[string]bool{
		"abcdef123456":  true,  // 12 hex chars
		"ABCDEF":        false, // uppercase not accepted
		"redpanda":      false, // contains non-hex
		"abc":           false, // too short
		"":              false,
		"abcdef12345g":  false, // 'g' not hex
		"0000000000000": true,  // all-zero but valid length
	}
	for input, want := range cases {
		if got := looksLikeImageID(input); got != want {
			t.Errorf("looksLikeImageID(%q) = %v, want %v", input, got, want)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
