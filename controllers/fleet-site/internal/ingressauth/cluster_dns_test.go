package ingressauth

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestPortalHost(t *testing.T) {
	if got := PortalHost("apps.umkim.com"); got != "auth.apps.umkim.com" {
		t.Fatalf("PortalHost = %q", got)
	}
}

func TestLLDAPWebUIHost(t *testing.T) {
	if got := LLDAPWebUIHost("apps.umkim.com"); got != "lldap.apps.umkim.com" {
		t.Fatalf("LLDAPWebUIHost = %q", got)
	}
}

func TestAccessDomainsForZones(t *testing.T) {
	got := AccessDomainsForZones([]string{"apps.umkim.com", "apps.umkim.com"})
	if len(got) != 2 {
		t.Fatalf("expected 2 domains, got %v", got)
	}
}

func TestMatchedZones(t *testing.T) {
	site := &fleetv1alpha1.FleetSite{
		Spec: fleetv1alpha1.FleetSiteSpec{
			Edge: fleetv1alpha1.FleetSiteEdgeSpec{
				Domains: []fleetv1alpha1.FleetSiteDomainSpec{
					{ID: "homelab", Zone: "apps.umkim.com", Wildcard: "*.apps.umkim.com"},
				},
			},
		},
	}
	zones := matchedZones(site, []string{"homelab"})
	if len(zones) != 1 || zones[0] != "apps.umkim.com" {
		t.Fatalf("zones = %v", zones)
	}
}

func TestDNSFromFCJStatus(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			IngressAuth: fleetv1alpha1.FleetClusterJoinIngressAuthStatus{
				PortalHost:    "auth.apps.umkim.com",
				AccessDomains: []string{"*.apps.umkim.com"},
			},
		},
	}
	dns := DNSFromFCJStatus(fcj)
	if dns.PortalHost != "auth.apps.umkim.com" || dns.PrimaryZone != "apps.umkim.com" {
		t.Fatalf("dns = %+v", dns)
	}
}
