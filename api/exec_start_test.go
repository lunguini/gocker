package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// createExec registers an exec instance via the API and returns its ID.
func createExec(t *testing.T, srv *Server) string {
	t.Helper()
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"Cmd":["true"],"AttachStdout":true}`)
	req := httptest.NewRequest("POST", "/containers/web/exec", body)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("exec create failed: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct{ Id string }
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Id
}

func TestExecStart_MalformedBodyReturns400(t *testing.T) {
	srv := NewServer(&stubRuntime{}, "")
	id := createExec(t, srv)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/exec/"+id+"/start", strings.NewReader("{not json"))
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed body, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestExecStart_EmptyBodyStillWorks(t *testing.T) {
	srv := NewServer(&stubRuntime{}, "")
	id := createExec(t, srv)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/exec/"+id+"/start", nil)
	srv.ServeHTTP(rec, req)
	// Empty body means non-detach, which requires connection hijack —
	// httptest recorders can't hijack, so a 500 mentioning hijack is the
	// expected "got past body validation" signal. 400 would be a regression.
	if rec.Code == http.StatusBadRequest {
		t.Errorf("empty body must not be rejected as malformed, got 400: %s", rec.Body.String())
	}
}
