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

func TestResolveClusterIngress_reverseProxyAuth(t *testing.T) {
	withZone := fleetv1alpha1.FleetSiteSpec{
		Edge: fleetv1alpha1.FleetSiteEdgeSpec{
			Ingress: []fleetv1alpha1.FleetSiteIngressSpec{{
				ID: "rp1", Method: fleetv1alpha1.IngressMethodReverseProxy, Zone: "otthol.hu",
			}},
		},
	}
	ci, err := ResolveClusterIngress(withZone, "rp1")
	if err != nil {
		t.Fatal(err)
	}
	if !ci.AllowsAuth {
		t.Fatalf("reverse_proxy with a zone should allow ingress auth: %#v", ci)
	}
	if ci.AllowsSpokeACME {
		t.Fatal("reverse_proxy must not enable spoke ACME")
	}
	if ci.TlsPolicy != fleetv1alpha1.TLSPolicyExternal {
		t.Fatalf("reverse_proxy TLS policy = %q, want external", ci.TlsPolicy)
	}

	noZone := fleetv1alpha1.FleetSiteSpec{
		Edge: fleetv1alpha1.FleetSiteEdgeSpec{
			Ingress: []fleetv1alpha1.FleetSiteIngressSpec{{
				ID: "rp1", Method: fleetv1alpha1.IngressMethodReverseProxy,
			}},
		},
	}
	ci, err = ResolveClusterIngress(noZone, "rp1")
	if err != nil {
		t.Fatal(err)
	}
	if ci.AllowsAuth {
		t.Fatal("reverse_proxy without a zone must not allow ingress auth")
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

func TestResolveClusterIngress_dnsModeEmptyAddress(t *testing.T) {
	site := fleetv1alpha1.FleetSiteSpec{
		Edge: fleetv1alpha1.FleetSiteEdgeSpec{
			WanEgress: &fleetv1alpha1.FleetSiteWanEgressSpec{Enabled: true},
			Ingress: []fleetv1alpha1.FleetSiteIngressSpec{{
				ID:     "pf1",
				Method: fleetv1alpha1.IngressMethodPortForward,
				Zone:   "demo.example.com",
			}},
		},
	}
	ci, err := ResolveClusterIngress(site, "pf1")
	if err != nil {
		t.Fatal(err)
	}
	if ci.WanAddressMode != fleetv1alpha1.WanAddressModeDNS {
		t.Fatalf("mode=%q", ci.WanAddressMode)
	}
	if got := EdgeWANIP(ci); got != "" {
		t.Fatalf("edge wan ip=%q want empty", got)
	}
}
