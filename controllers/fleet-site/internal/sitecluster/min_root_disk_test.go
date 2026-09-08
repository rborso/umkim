package sitecluster

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestEffectiveMinRootDiskGi(t *testing.T) {
	if got := effectiveMinRootDiskGi(fleetv1alpha1.SiteHostRecord{Kind: fleetv1alpha1.SiteHostKindVM}); got != ProvisionedVMMinRootDiskGi {
		t.Fatalf("vm host: got %d want %d", got, ProvisionedVMMinRootDiskGi)
	}
	if got := effectiveMinRootDiskGi(fleetv1alpha1.SiteHostRecord{Kind: fleetv1alpha1.SiteHostKindBareMetal}); got != 0 {
		t.Fatalf("bare metal: got %d want 0", got)
	}
}
