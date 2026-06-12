package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/lunguini/gocker/engine"
)

// isNotFoundErr reports whether a runtime error means "resource doesn't
// exist". The engine layer classifies CLI stderr into engine.ErrNotFound;
// the string matching below remains as a fallback for errors that arrive
// without classification (e.g. proxied through the shared VM).
func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, engine.ErrNotFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"not found",
		"no such",
		"does not exist",
		"unknown image",
		"unknown container",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// writeRuntimeError maps a runtime error to the Docker API convention:
// 404 with a "No such <resource>" message when the resource is missing,
// 500 otherwise.
func writeRuntimeError(w http.ResponseWriter, err error, resource, name string) {
	if isNotFoundErr(err) {
		writeError(w, http.StatusNotFound, "No such "+resource+": "+name)
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}
