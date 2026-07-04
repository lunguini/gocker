package cmd

import (
	"strings"
	"testing"
)

func TestRenderDiagnostics_HealthyWhenNoFailures(t *testing.T) {
	checks := []diagCheck{
		{name: "Platform", detail: "darwin", status: statusOK},
		{name: "Daemon", detail: "not running", status: statusWarn},
	}
	out, healthy := renderDiagnostics(checks)
	if !healthy {
		t.Error("warnings alone should not mark unhealthy")
	}
	if !strings.Contains(out, "Platform") || !strings.Contains(out, "darwin") {
		t.Errorf("output missing check content: %q", out)
	}
}

func TestRenderDiagnostics_UnhealthyOnFailure(t *testing.T) {
	checks := []diagCheck{
		{name: "Container binary", detail: "missing", status: statusFail},
	}
	_, healthy := renderDiagnostics(checks)
	if healthy {
		t.Error("a failing check must mark the report unhealthy")
	}
}
