package httpapi

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestIsSupportedProviderKind(t *testing.T) {
	for _, kind := range []string{
		fleetv1alpha1.ProviderKindK8s,
		fleetv1alpha1.ProviderKindHelm,
		fleetv1alpha1.ProviderKindKubevirt,
	} {
		if !isSupportedProviderKind(kind) {
			t.Fatalf("expected supported: %s", kind)
		}
	}
	if isSupportedProviderKind("unknown") {
		t.Fatal("unexpected support for unknown")
	}
}

func TestProviderDisplayName(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Spec: fleetv1alpha1.FleetClusterJoinSpec{ClusterName: "c1"},
	}
	if got := providerDisplayName(fcj, "c1", fleetv1alpha1.ProviderKindK8s); got != "k8s-c1" {
		t.Fatalf("k8s provider name %q", got)
	}
	if got := providerDisplayName(fcj, "c1", fleetv1alpha1.ProviderKindKubevirt); got != "kubevirt-c1" {
		t.Fatalf("kubevirt provider name %q", got)
	}
}
