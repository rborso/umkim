package yaidap

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestValidateActionForKind(t *testing.T) {
	cases := []struct {
		action  string
		kind    string
		allowed bool
	}{
		{"create_vm", fleetv1alpha1.SiteHostKindVM, true},
		{"create_vm", fleetv1alpha1.SiteHostKindHypervisor, false},
		{"destroy", fleetv1alpha1.SiteHostKindVM, true},
		{"destroy", fleetv1alpha1.SiteHostKindBareMetal, false},
		{"discover", fleetv1alpha1.SiteHostKindHypervisor, true},
		{"discover", fleetv1alpha1.SiteHostKindBareMetal, true},
		{"discover", fleetv1alpha1.SiteHostKindVM, false},
		{"discover_pxe", fleetv1alpha1.SiteHostKindHypervisor, true},
		{"discover_pxe", fleetv1alpha1.SiteHostKindBareMetal, true},
		{"install_bm", fleetv1alpha1.SiteHostKindBareMetal, true},
		{"install_bm", fleetv1alpha1.SiteHostKindHypervisor, true},
		{"install_bm", fleetv1alpha1.SiteHostKindVM, false},
		{"prepare_hypervisor", fleetv1alpha1.SiteHostKindHypervisor, true},
		{"prepare_hypervisor", fleetv1alpha1.SiteHostKindBareMetal, false},
		{"prepare_gpu_passthrough", fleetv1alpha1.SiteHostKindHypervisor, true},
		{"prepare_hypervisor_gpu", fleetv1alpha1.SiteHostKindHypervisor, true},
	}
	for _, tc := range cases {
		err := ValidateActionForKind(tc.action, tc.kind)
		if tc.allowed && err != nil {
			t.Fatalf("%s on %s: unexpected error %v", tc.action, tc.kind, err)
		}
		if !tc.allowed && err == nil {
			t.Fatalf("%s on %s: expected error", tc.action, tc.kind)
		}
	}
}
