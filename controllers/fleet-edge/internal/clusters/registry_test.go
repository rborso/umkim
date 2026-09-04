package clusters

import "testing"

func TestSpokeACMEEnabled(t *testing.T) {
	c := Cluster{Method: methodPortForward, TlsPolicy: TLSPolicyACMECluster}
	if !c.SpokeACMEEnabled(true) {
		t.Fatal("expected acme_cluster with global ACME enabled")
	}
	if c.SpokeACMEEnabled(false) {
		t.Fatal("expected false when global ACME disabled")
	}
	if c := (Cluster{TlsPolicy: "external"}); c.SpokeACMEEnabled(true) {
		t.Fatal("expected false for external tls policy")
	}
}
