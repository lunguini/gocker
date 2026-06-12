package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/lunguini/gocker/engine"
)

func TestPull_UnauthorizedSuggestsLogin(t *testing.T) {
	mock := &engine.MockRuntime{
		ImagePullFunc: func(ctx context.Context, image string, opts engine.ImagePullOpts) error {
			return fmt.Errorf("401 Unauthorized: %w", engine.ErrUnauthorized)
		},
	}
	cmd := newPullCmd(mock)
	err := cmd.Run(context.Background(), []string{"pull", "private/image"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gocker login") {
		t.Errorf("unauthorized pull should suggest 'gocker login', got: %v", err)
	}
}

func TestPull_OtherErrorsNotDecorated(t *testing.T) {
	mock := &engine.MockRuntime{
		ImagePullFunc: func(ctx context.Context, image string, opts engine.ImagePullOpts) error {
			return fmt.Errorf("manifest unknown")
		},
	}
	cmd := newPullCmd(mock)
	err := cmd.Run(context.Background(), []string{"pull", "ghost/image"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "gocker login") {
		t.Errorf("non-auth error must not suggest login, got: %v", err)
	}
}
