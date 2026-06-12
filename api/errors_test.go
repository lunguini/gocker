package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsNotFoundErr(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("exit status 1: \"web\": not found"), true},
		{errors.New("No such container: web"), true},
		{errors.New("no such container web"), true},
		{errors.New("container does not exist"), true},
		{errors.New("unknown image: foo"), true},
		{errors.New("XPC connection error"), false},
		{errors.New("permission denied"), false},
	}
	for _, c := range cases {
		if got := isNotFoundErr(c.err); got != c.want {
			t.Errorf("isNotFoundErr(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestContainerLifecycle_NotFoundReturns404(t *testing.T) {
	notFound := fmt.Errorf(`"ghost": not found`)
	stub := &stubRuntime{
		containerStart:  func(ctx context.Context, id string) error { return notFound },
		containerStop:   func(ctx context.Context, id string) error { return notFound },
		containerRemove: func(ctx context.Context, id string, force bool) error { return notFound },
	}
	srv := NewServer(stub, "")

	reqs := []struct {
		method, path string
	}{
		{"POST", "/containers/ghost/start"},
		{"POST", "/containers/ghost/stop"},
		{"POST", "/containers/ghost/kill"},
		{"DELETE", "/containers/ghost"},
	}
	for _, r := range reqs {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(r.method, r.path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: expected 404, got %d (body: %s)", r.method, r.path, rec.Code, rec.Body.String())
		}
	}
}

func TestContainerLifecycle_OtherErrorsReturn500(t *testing.T) {
	boom := errors.New("XPC connection error")
	stub := &stubRuntime{
		containerStart: func(ctx context.Context, id string) error { return boom },
	}
	srv := NewServer(stub, "")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("POST", "/containers/web/start", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for non-not-found error, got %d", rec.Code)
	}
}

func TestVolumeRemove_NotFoundReturns404(t *testing.T) {
	stub := &stubRuntime{
		volumeRemove: func(ctx context.Context, name string) error {
			return fmt.Errorf(`volume "ghost" not found`)
		},
	}
	srv := NewServer(stub, "")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("DELETE", "/volumes/ghost", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestNetworkRemove_NotFoundReturns404(t *testing.T) {
	stub := &stubRuntime{
		networkRemove: func(ctx context.Context, name string) error {
			return fmt.Errorf(`network "ghost" not found`)
		},
	}
	srv := NewServer(stub, "")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("DELETE", "/networks/ghost", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}
