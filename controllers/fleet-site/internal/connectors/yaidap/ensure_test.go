package yaidap

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestGuestSpecFromHostBareMetal(t *testing.T) {
	spec := GuestSpecFromHost(fleetv1alpha1.SiteHostRecord{
		Name:        "bm01",
		Kind:        fleetv1alpha1.SiteHostKindBareMetal,
		AnsibleHost: "203.0.113.50",
		OSFamily:    "rocky",
	})
	if spec.Kind != fleetv1alpha1.SiteHostKindBareMetal {
		t.Fatalf("kind=%q", spec.Kind)
	}
	if spec.VmLocation != "" {
		t.Fatalf("vm_location=%q want empty", spec.VmLocation)
	}
	if spec.ProvisioningProvider != "bare_metal" {
		t.Fatalf("provider=%q", spec.ProvisioningProvider)
	}
}

func TestContainsInventorySynced(t *testing.T) {
	if !containsInventorySynced("TASK [Emit inventory sync marker] ok: [jumphost] => FLEET_INVENTORY_SYNCED=1") {
		t.Fatal("expected marker detection")
	}
}
