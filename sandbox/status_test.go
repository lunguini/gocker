package sandbox

import (
	"context"
	"errors"
	"testing"

	"github.com/lunguini/gocker/engine"
)

func TestGetContainerStatus(t *testing.T) {
	cases := []struct {
		name string
		data string
		err  error
		want string
	}{
		{"json object with status", `{"status":"running"}`, nil, "running"},
		{"json array with status", `[{"status":"running","configuration":{"id":"x"}}]`, nil, "running"},
		{"nested State.Status", `{"State":{"Status":"exited"}}`, nil, "exited"},
		{"inspect error returns empty", ``, errors.New("not found"), ""},
		{"empty array returns empty", `[]`, nil, ""},
		{"no status field returns empty", `{"configuration":{"id":"x"}}`, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &engine.MockRuntime{
				ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
					return []byte(c.data), c.err
				},
			}
			m := NewManager(rt)
			got := m.getContainerStatus(context.Background(), "some-id")
			if got != c.want {
				t.Errorf("getContainerStatus() = %q, want %q", got, c.want)
			}
		})
	}
}
