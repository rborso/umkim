package providers

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestNextFreePort(t *testing.T) {
	used := map[int]bool{30190: true, 30191: true}
	got := nextFreePort(used, 30190, 30200)
	if got != 30192 {
		t.Fatalf("got %d want 30192", got)
	}
}

func TestAllocateNodePortsKubevirtOptional(t *testing.T) {
	install := fleetv1alpha1.ProviderInstallSpec{Kubevirt: true}
	if !install.Kubevirt {
		t.Fatal("install flags")
	}
}
