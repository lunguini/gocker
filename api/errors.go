package api

import (
	"net/http"
	"strings"
)

// isNotFoundErr reports whether a runtime error means "resource doesn't
// exist". Both Apple's container CLI and nerdctl only expose this through
// error text, so string matching is the only option — keep every known
// phrasing here so handlers don't grow their own variants.
func isNotFoundErr(err error) bool {
	if err == nil {
		return false
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
