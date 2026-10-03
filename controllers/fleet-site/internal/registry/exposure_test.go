package registry

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestEffectiveClusterExposureMode(t *testing.T) {
	mode := EffectiveClusterExposureMode(ClusterExposureInput{Method: "reverse_proxy"}, "")
	if mode != fleetv1alpha1.ExposureModeReverseProxy {
		t.Fatalf("got %q", mode)
	}
	mode = EffectiveClusterExposureMode(ClusterExposureInput{DirectIngress: true}, "")
	if mode != fleetv1alpha1.ExposureModePortForward {
		t.Fatalf("legacy got %q", mode)
	}
	mode = EffectiveClusterExposureMode(ClusterExposureInput{Method: "port_forward"}, "")
	if mode != fleetv1alpha1.ExposureModePortForward {
		t.Fatalf("port_forward got %q", mode)
	}
}

func TestSpokeACMEEnabled(t *testing.T) {
	if SpokeACMEEnabled(fleetv1alpha1.TLSPolicyExternal) {
		t.Fatal("external should disable spoke ACME")
	}
	if !SpokeACMEEnabled(fleetv1alpha1.TLSPolicyACMECluster) {
		t.Fatal("acme_cluster should enable spoke ACME")
	}
}
