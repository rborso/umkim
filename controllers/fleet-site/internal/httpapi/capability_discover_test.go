package httpapi

import (
	"testing"
	"time"

	"github.com/rborso/umkim/controllers/fleet-site/internal/capdiscover"
)

func TestDiscoveryTerminalAfterRequest(t *testing.T) {
	requested := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

	stale := &capdiscover.Result{
		Phase:       capdiscover.PhaseSucceeded,
		CompletedAt: "2026-10-02T00:22:50Z",
	}
	if discoveryTerminalAfterRequest(stale, requested) {
		t.Fatal("stale Succeeded before request time must not count as terminal")
	}

	fresh := &capdiscover.Result{
		Phase:       capdiscover.PhaseSucceeded,
		CompletedAt: "2026-10-02T12:05:00Z",
	}
	if !discoveryTerminalAfterRequest(fresh, requested) {
		t.Fatal("Succeeded after request time must count as terminal")
	}

	running := &capdiscover.Result{Phase: capdiscover.PhaseRunning}
	if discoveryTerminalAfterRequest(running, requested) {
		t.Fatal("Running is not terminal")
	}

	failed := &capdiscover.Result{Phase: capdiscover.PhaseFailed}
	if !discoveryTerminalAfterRequest(failed, requested) {
		t.Fatal("Failed without completed_at should count as terminal")
	}
}
