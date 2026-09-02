package reconcile

import (
	"testing"
)

func TestDialPublicPortInvalidIP(t *testing.T) {
	msg := dialPublicPort(t.Context(), "203.0.113.199", "59999")
	if msg == "" {
		t.Fatal("expected dial failure for closed port")
	}
}

func TestProbeHTTPChallengePathEmptyHost(t *testing.T) {
	ok, msg := validateSpokeConnectivity(t.Context(), "")
	if ok || msg == "" {
		t.Fatalf("expected failure for empty host, ok=%v msg=%q", ok, msg)
	}
}
