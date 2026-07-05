package api

import (
	"context"
	"errors"
	"testing"
)

func TestContainerState(t *testing.T) {
	cases := []struct {
		name    string
		data    string
		err     error
		want    string
		wantErr bool
	}{
		{"flat lowercase status", `{"status":"Running"}`, nil, "running", false},
		{"nested State.Status", `{"State":{"Status":"Exited"}}`, nil, "exited", false},
		{"array of objects", `[{"status":"Running"}]`, nil, "running", false},
		{"inspect transport error propagates", ``, errors.New("boom"), "", true},
		{"unparseable payload errors", `not json at all and no array either`, nil, "", true},
		{"no status field returns empty, no error", `{"configuration":{"id":"x"}}`, nil, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := NewServer(&stubRuntime{
				containerInspect: func(ctx context.Context, nameOrID string) ([]byte, error) {
					return []byte(c.data), c.err
				},
			}, "")
			got, err := srv.containerState(context.Background(), "some-id")
			if (err != nil) != c.wantErr {
				t.Fatalf("containerState() error = %v, wantErr %v", err, c.wantErr)
			}
			if got != c.want {
				t.Errorf("containerState() = %q, want %q", got, c.want)
			}
		})
	}
}
