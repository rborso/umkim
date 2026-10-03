package reconcile

import (
	"context"
	"testing"

	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
)

func TestValidateSiteDNS_dnsModeIgnoresStaleIP(t *testing.T) {
	r := &RouteReconciler{}
	regCluster := clusters.Cluster{
		Method:         "port_forward",
		EdgeWANIP:      "203.0.113.1",
		WanAddressMode: "dns",
		UpstreamHost:   "127.0.0.1",
		TraefikHTTPPort: 59999,
	}
	ok, msg := r.validateSiteDNS(context.Background(), "localhost", regCluster)
	if ok {
		t.Fatal("expected failure without reachable connectivity")
	}
	if msg == "" {
		t.Fatal("expected message")
	}
}

func TestValidateSiteDNS_fixedModeRequiresMatch(t *testing.T) {
	r := &RouteReconciler{}
	regCluster := clusters.Cluster{
		Method:         "port_forward",
		EdgeWANIP:      "203.0.113.199",
		WanAddressMode: "fixed",
	}
	ok, msg := r.validateSiteDNS(context.Background(), "localhost", regCluster)
	if ok {
		t.Fatal("expected DNS mismatch failure")
	}
	if msg == "" || msg == "hostname localhost did not resolve" {
		// localhost resolves but not to 203.0.113.199
	}
}

func TestSpokeProbeHost_dnsModePrefersDNS(t *testing.T) {
	regCluster := clusters.Cluster{
		Method:         "port_forward",
		EdgeWANIP:      "203.0.113.1",
		WanAddressMode: "dns",
	}
	host := spokeProbeHost(context.Background(), "localhost", regCluster)
	if host == "203.0.113.1" {
		t.Fatalf("dns mode should not prefer stale edge_wan_ip, got %q", host)
	}
}
