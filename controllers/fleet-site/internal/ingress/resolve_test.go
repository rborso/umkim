package ingress

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestResolveClusterIngress_portForward(t *testing.T) {
	site := fleetv1alpha1.FleetSiteSpec{
		Edge: fleetv1alpha1.FleetSiteEdgeSpec{
			Ingress: []fleetv1alpha1.FleetSiteIngressSpec{{
				ID:      "pf1",
				Method:  fleetv1alpha1.IngressMethodPortForward,
				Address: "203.0.113.1",
				Zone:    "demo.example.com",
			}},
		},
	}
	ci, err := ResolveClusterIngress(site, "pf1")
	if err != nil {
		t.Fatal(err)
	}
	if ci.TlsPolicy != fleetv1alpha1.TLSPolicyACMECluster || !ci.AllowsAuth || !ci.AllowsSpokeACME {
		t.Fatalf("unexpected pf flags: %+v", ci)
	}
	if got := DefaultSiteHost("whoami", ci); got != "whoami.demo.example.com" {
		t.Fatalf("default host %q", got)
	}
}

func TestValidateBinding_portForwardCardinality(t *testing.T) {
	site := fleetv1alpha1.FleetSiteSpec{
		Edge: fleetv1alpha1.FleetSiteEdgeSpec{
			Ingress: []fleetv1alpha1.FleetSiteIngressSpec{{
				ID: "pf1", Method: fleetv1alpha1.IngressMethodPortForward,
				Address: "1.2.3.4", Zone: "z.example.com",
			}},
		},
	}
	existing := map[string][]string{"pf1": {"other"}}
	if err := ValidateBinding(site, "pf1", "new", existing); err == nil {
		t.Fatal("expected cardinality error")
	}
}

func TestValidateBinding_reverseProxyShared(t *testing.T) {
	site := fleetv1alpha1.FleetSiteSpec{
		Edge: fleetv1alpha1.FleetSiteEdgeSpec{
			Ingress: []fleetv1alpha1.FleetSiteIngressSpec{{
				ID: "rp1", Method: fleetv1alpha1.IngressMethodReverseProxy,
			}},
		},
	}
	existing := map[string][]string{"rp1": {"a", "b"}}
	if err := ValidateBinding(site, "rp1", "c", existing); err != nil {
		t.Fatal(err)
	}
}

func TestHostMatchesWildcard(t *testing.T) {
	if !HostMatchesWildcard("app.demo.example.com", "*.demo.example.com") {
		t.Fatal("expected match")
	}
	if HostMatchesWildcard("other.example.com", "*.demo.example.com") {
		t.Fatal("expected no match")
	}
}
