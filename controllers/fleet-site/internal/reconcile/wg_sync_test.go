package reconcile

import (
	"testing"

	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
)

func TestPeerSetHashStable(t *testing.T) {
	add := []jobs.WGPeer{{PublicKey: "pk1", AllowedIP: "10.10.0.2/32"}}
	remove := []jobs.WGPeer{{PublicKey: "pk0", AllowedIP: "10.10.0.3/32"}}
	h1 := peerSetHash(add, remove)
	h2 := peerSetHash([]jobs.WGPeer{{PublicKey: "pk1", AllowedIP: "10.10.0.2/32"}}, []jobs.WGPeer{{PublicKey: "pk0", AllowedIP: "10.10.0.3/32"}})
	if h1 != h2 {
		t.Fatalf("expected stable hash, got %q vs %q", h1, h2)
	}
	if peerSetHash(add, nil) == h1 {
		t.Fatal("expected different hash when remove peers differ")
	}
}

func TestClusterWgSyncJobName(t *testing.T) {
	if got := clusterWgSyncJobName("lab-single-0"); got != "fleet-cluster-wg-lab-single-0" {
		t.Fatalf("unexpected job name: %q", got)
	}
}

func TestClusterNodeOpJobName(t *testing.T) {
	if got := clusterNodeOpJobName("umkim-prov-0"); got != "fleet-cluster-nodeop-umkim-prov-0" {
		t.Fatalf("unexpected job name: %q", got)
	}
}
