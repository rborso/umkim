package reconcile

import (
	"testing"

	"github.com/rborso/umkim/controllers/fleet-edge/internal/config"
)

func TestEffectiveGatewayProbeHost(t *testing.T) {
	cfg := config.Config{GatewayProbeHost: "localhost", GatewayWANIP: "203.0.113.10"}
	if got := effectiveGatewayProbeHost(cfg); got != "203.0.113.10" {
		t.Fatalf("want WAN fallback, got %q", got)
	}
	cfg = config.Config{GatewayProbeHost: "", GatewayWANIP: ""}
	if got := effectiveGatewayProbeHost(cfg); got != wgHubProbeFallback {
		t.Fatalf("want wg hub fallback, got %q", got)
	}
}

func TestEffectiveGatewayWANIP(t *testing.T) {
	cfg := config.Config{GatewayWANIP: "localhost", GatewayProbeHost: "198.51.100.5"}
	if got := effectiveGatewayWANIP(cfg); got != "198.51.100.5" {
		t.Fatalf("want probe fallback, got %q", got)
	}
	cfg = config.Config{}
	if got := effectiveGatewayWANIP(cfg); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestInvalidProbeHost(t *testing.T) {
	for _, h := range []string{"", "127.0.0.1", "localhost", " LOCALHOST "} {
		if !invalidProbeHost(h) {
			t.Fatalf("%q should be invalid", h)
		}
	}
	if invalidProbeHost("203.0.113.1") {
		t.Fatal("public IP should be valid")
	}
}

func TestEffectiveGatewayProbeHostTrims(t *testing.T) {
	cfg := config.Config{GatewayProbeHost: "  10.1.2.3  "}
	if got := effectiveGatewayProbeHost(cfg); got != "10.1.2.3" {
		t.Fatalf("got %q", got)
	}
}
