package reconcile

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestMergeEdgeClusterEntryForJoinPreservesConsoleFields(t *testing.T) {
	prev := edgeClusterUpsert{
		IngressEndpointID: "router-wan",
		DomainBindings:    []string{"primary"},
		Management:        true,
		TraefikHTTPPort:   30080,
	}
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			SiteId: "homelab-ext",
			Edge: fleetv1alpha1.FleetClusterJoinEdgeSpec{
				ExposureMode: "inherit",
			},
		},
	}
	got := mergeEdgeClusterEntryForJoin(prev, fcj, fleetv1alpha1.ExposureModePortForward, "10.10.0.12")
	if got.IngressEndpointID != "router-wan" {
		t.Fatalf("ingress_endpoint_id=%q want router-wan", got.IngressEndpointID)
	}
	if len(got.DomainBindings) != 1 || got.DomainBindings[0] != "primary" {
		t.Fatalf("domain_bindings=%v want [primary]", got.DomainBindings)
	}
	if !got.Management {
		t.Fatal("management flag should be preserved")
	}
	if got.ExposureMode != fleetv1alpha1.ExposureModePortForward {
		t.Fatalf("exposure_mode=%q want port_forward", got.ExposureMode)
	}
}

func TestMergeEdgeClusterEntryForJoinPrefersFCJSpec(t *testing.T) {
	prev := edgeClusterUpsert{
		IngressEndpointID: "old-endpoint",
		DomainBindings:    []string{"legacy"},
	}
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			SiteId: "homelab-ext",
			Edge: fleetv1alpha1.FleetClusterJoinEdgeSpec{
				ExposureMode:      "inherit",
				IngressEndpointId: "router-wan",
			},
			DomainBindings: []string{"primary"},
		},
	}
	got := mergeEdgeClusterEntryForJoin(prev, fcj, fleetv1alpha1.ExposureModePortForward, "10.10.0.12")
	if got.IngressEndpointID != "router-wan" {
		t.Fatalf("ingress_endpoint_id=%q want router-wan", got.IngressEndpointID)
	}
	if len(got.DomainBindings) != 1 || got.DomainBindings[0] != "primary" {
		t.Fatalf("domain_bindings=%v want [primary]", got.DomainBindings)
	}
}
