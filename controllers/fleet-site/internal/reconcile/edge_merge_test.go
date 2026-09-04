package reconcile

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestMergeEdgeClusterEntryForJoinPreservesConsoleFields(t *testing.T) {
	prev := edgeClusterUpsert{
		IngressID:       "homelab-pf",
		Management:      true,
		TraefikHTTPPort: 30080,
	}
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			SiteId: "homelab",
		},
	}
	site := &fleetv1alpha1.FleetSite{
		Spec: fleetv1alpha1.FleetSiteSpec{
			Edge: fleetv1alpha1.FleetSiteEdgeSpec{
				Ingress: []fleetv1alpha1.FleetSiteIngressSpec{{
					ID: "homelab-pf", Method: fleetv1alpha1.IngressMethodPortForward,
					Address: "203.0.113.1", Zone: "homelab.example.com",
				}},
			},
		},
	}
	got := mergeEdgeClusterEntryForJoin(prev, fcj, site, "10.10.0.12")
	if got.IngressID != "homelab-pf" {
		t.Fatalf("ingress_id=%q want homelab-pf", got.IngressID)
	}
	if !got.Management {
		t.Fatal("management flag should be preserved")
	}
	if got.Method != fleetv1alpha1.IngressMethodPortForward {
		t.Fatalf("method=%q want port_forward", got.Method)
	}
	if got.DNSZone != "homelab.example.com" {
		t.Fatalf("dns_zone=%q", got.DNSZone)
	}
}

func TestMergeEdgeClusterEntryForJoinPrefersFCJSpec(t *testing.T) {
	prev := edgeClusterUpsert{IngressID: "old-ingress"}
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			SiteId: "homelab",
			Edge: fleetv1alpha1.FleetClusterJoinEdgeSpec{
				IngressId: "edge-rp",
			},
		},
	}
	site := &fleetv1alpha1.FleetSite{
		Spec: fleetv1alpha1.FleetSiteSpec{
			Edge: fleetv1alpha1.FleetSiteEdgeSpec{
				Ingress: []fleetv1alpha1.FleetSiteIngressSpec{
					{ID: "homelab-pf", Method: fleetv1alpha1.IngressMethodPortForward, Address: "203.0.113.1", Zone: "pf.example.com"},
					{ID: "edge-rp", Method: fleetv1alpha1.IngressMethodReverseProxy},
				},
			},
		},
	}
	got := mergeEdgeClusterEntryForJoin(prev, fcj, site, "10.10.0.12")
	if got.IngressID != "edge-rp" {
		t.Fatalf("ingress_id=%q want edge-rp", got.IngressID)
	}
	if got.Method != fleetv1alpha1.IngressMethodReverseProxy {
		t.Fatalf("method=%q want reverse_proxy", got.Method)
	}
	if got.TlsPolicy != fleetv1alpha1.TLSPolicyExternal {
		t.Fatalf("tls_policy=%q want external", got.TlsPolicy)
	}
}
