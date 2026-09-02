package traefik

import (
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
)

func authOpts() BuildOptions {
	return BuildOptions{
		AutheliaEnabled:        true,
		AutheliaForwardAuthURL: "http://authelia.fleet-ingress-auth.svc.cluster.local:9091/api/authz/forward-auth",
		AutheliaPortalHost:     "auth.mgmt.umkim.com",
		AutheliaServiceURL:     "http://authelia.fleet-ingress-auth.svc.cluster.local:9091",
	}
}

func TestBuildEmptyRoutesOmitsRouterMaps(t *testing.T) {
	out, err := Build(nil, clusters.Registry{}, false, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "routers:") || strings.Contains(s, "services:") {
		t.Fatalf("empty build must not emit routers/services keys (Traefik 3): %s", s)
	}
}

func TestBuildAuthPortalRoutesWhenEnabled(t *testing.T) {
	out, err := Build(nil, clusters.Registry{}, true, authOpts())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "fleet-authelia-portal") {
		t.Fatalf("expected authelia portal routes: %s", s)
	}
	if !strings.Contains(s, "auth.mgmt.umkim.com") {
		t.Fatalf("expected portal host rule: %s", s)
	}
	if !strings.Contains(s, "fleet-authelia-portal-tls") {
		t.Fatalf("expected authelia portal TLS router for ACME issuance: %s", s)
	}
	if !strings.Contains(s, "fleet-edge-authelia") {
		t.Fatalf("expected authelia middleware: %s", s)
	}
}

func TestBuildHubRoute(t *testing.T) {
	routes := []fleetv1alpha1.FleetAppRoute{{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:    "abc-123",
			Cluster:       "virt02",
			Mode:          fleetv1alpha1.RouteModeHub,
			EffectiveMode: fleetv1alpha1.RouteModeHub,
			HubHost:       "myapp.mgmt.umkim.com",
			RequireAuth:   true,
		},
	}}
	reg := clusters.Registry{
		Clusters: map[string]clusters.Cluster{
			"virt02": {DirectIngress: true, UpstreamHost: "10.10.0.3", TraefikHTTPPort: 30080},
		},
	}
	out, err := Build(routes, reg, false, authOpts())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "myapp.mgmt.umkim.com") {
		t.Fatalf("missing host rule: %s", s)
	}
	if strings.Contains(s, "acme-challenge") {
		t.Fatalf("non-ACME build must not exclude acme-challenge: %s", s)
	}
	if !strings.Contains(s, "10.10.0.3:30080") {
		t.Fatalf("missing upstream: %s", s)
	}
	mids := hubRouterMiddlewares(fleetv1alpha1.RouteModeHub, false, true, authOpts(), false)
	if len(mids) != 2 || mids[1] != "fleet-edge-authelia" {
		t.Fatalf("hub route with RequireAuth should chain authelia: %v", mids)
	}
	midsHTTPACME := hubRouterMiddlewares(fleetv1alpha1.RouteModeHub, false, true, authOpts(), true)
	if len(midsHTTPACME) != 1 || midsHTTPACME[0] != "fleet-edge-redirect-https" {
		t.Fatalf("hub HTTP route with ACME should redirect to HTTPS: %v", midsHTTPACME)
	}
	midsPending := hubRouterMiddlewares(fleetv1alpha1.RouteModeHub, false, true, BuildOptions{CertsReady: map[string]bool{}}, true)
	if len(midsPending) != 1 || midsPending[0] != "fleet-edge-redirect-https" {
		t.Fatalf("hub HTTP route with pending cert must redirect to HTTPS: %v", midsPending)
	}
}

func TestBuildDeferredHTTPSOmitsTLSRouter(t *testing.T) {
	routes := []fleetv1alpha1.FleetAppRoute{{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:    "abc-123",
			Cluster:       "virt02",
			Mode:          fleetv1alpha1.RouteModeHub,
			EffectiveMode: fleetv1alpha1.RouteModeHub,
			HubHost:       "pending.mgmt.umkim.com",
		},
	}}
	reg := clusters.Registry{
		Clusters: map[string]clusters.Cluster{
			"virt02": {DirectIngress: true, UpstreamHost: "10.10.0.3", TraefikHTTPPort: 30080},
		},
	}
	out, err := Build(routes, reg, true, BuildOptions{CertsReady: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "certResolver: letsencrypt") {
		t.Fatalf("ACME hub route must emit TLS router before cert exists: %s", s)
	}
	if !strings.Contains(s, "fleet-edge-redirect-https") {
		t.Fatalf("ACME hub HTTP must redirect to HTTPS: %s", s)
	}
	if !strings.Contains(s, "pending.mgmt.umkim.com") {
		t.Fatalf("expected HTTP router: %s", s)
	}
}

func TestBuildHubRouteACMESkipsChallengeRedirect(t *testing.T) {
	routes := []fleetv1alpha1.FleetAppRoute{{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:    "abc-123",
			Cluster:       "virt02",
			Mode:          fleetv1alpha1.RouteModeHub,
			EffectiveMode: fleetv1alpha1.RouteModeHub,
			HubHost:       "myapp.mgmt.umkim.com",
		},
	}}
	reg := clusters.Registry{
		Clusters: map[string]clusters.Cluster{
			"virt02": {DirectIngress: true, UpstreamHost: "10.10.0.3", TraefikHTTPPort: 30080},
		},
	}
	out, err := Build(routes, reg, true, BuildOptions{
		CertsReady: map[string]bool{"myapp.mgmt.umkim.com": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "acme-challenge") {
		t.Fatalf("ACME HTTP router must exclude acme-challenge path: %s", s)
	}
	if !strings.Contains(s, "certResolver: letsencrypt") {
		t.Fatalf("ACME HTTPS router must use letsencrypt: %s", s)
	}
}

func TestBuildStandbyDualUpstreamHealthCheck(t *testing.T) {
	followFalse := false
	routes := []fleetv1alpha1.FleetAppRoute{{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:     "ms-1",
			Cluster:        "virt01",
			StandbyCluster: "virt02",
			FailoverMode:   fleetv1alpha1.FailoverModeStandby,
			Mode:           fleetv1alpha1.RouteModeHub,
			EffectiveMode:  fleetv1alpha1.RouteModeHub,
			HubHost:        "myapp.mgmt.umkim.com",
			HealthPath:     "/",
		},
	}}
	reg := clusters.Registry{
		Clusters: map[string]clusters.Cluster{
			"virt01": {UpstreamHost: "10.10.0.10", TraefikHTTPPort: 30080},
			"virt02": {UpstreamHost: "10.10.0.12", TraefikHTTPPort: 30080},
		},
	}
	out, err := Build(routes, reg, false, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		"hostname: myapp.mgmt.umkim.com",
		"status: 200",
		"followRedirects: false",
		"passHostHeader: true",
		"weight: 10",
		"10.10.0.10:30080",
		"10.10.0.12:30080",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	hc := failoverUpstreamHealthCheck(routes[0])
	if hc.Hostname != "myapp.mgmt.umkim.com" || hc.Status != 200 || *hc.FollowRedirects != followFalse {
		t.Fatalf("unexpected health check: %+v", hc)
	}
}

func TestBuildStandbyHubRouteUsesHTTPUpstreamOnACMECluster(t *testing.T) {
	routes := []fleetv1alpha1.FleetAppRoute{{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:     "ms-2",
			Cluster:        "virt01",
			StandbyCluster: "virt02",
			FailoverMode:   fleetv1alpha1.FailoverModeStandby,
			Mode:           fleetv1alpha1.RouteModeHub,
			EffectiveMode:  fleetv1alpha1.RouteModeHub,
			HubHost:        "myapp.mgmt.umkim.com",
		},
	}}
	reg := clusters.Registry{
		Clusters: map[string]clusters.Cluster{
			"virt01": {UpstreamHost: "10.10.0.10", TraefikHTTPPort: 30080},
			"virt02": {TlsPolicy: clusters.TLSPolicyACMECluster, UpstreamHost: "10.10.0.12", TraefikHTTPPort: 30080},
		},
	}
	out, err := Build(routes, reg, true, BuildOptions{
		CertsReady: map[string]bool{"myapp.mgmt.umkim.com": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		"http://10.10.0.12:30081",
		"http://10.10.0.10:30080",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "https://10.10.0.12:30443") {
		t.Fatalf("hub route must not use spoke HTTPS upstream:\n%s", s)
	}
	if strings.Contains(s, "serversTransports:") {
		t.Fatalf("serversTransports must be nested under http for Traefik v3:\n%s", s)
	}
}

func TestBuildHubRouteUsesHTTPUpstreamWhenEffectiveModeUnset(t *testing.T) {
	routes := []fleetv1alpha1.FleetAppRoute{{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID: "abc-123",
			Cluster:    "virt02",
			Mode:       fleetv1alpha1.RouteModeHub,
			HubHost:    "myapp.mgmt.umkim.com",
		},
	}}
	reg := clusters.Registry{
		Clusters: map[string]clusters.Cluster{
			"virt02": {TlsPolicy: clusters.TLSPolicyACMECluster, UpstreamHost: "10.10.0.12", TraefikHubHTTPPort: 30081},
		},
	}
	out, err := Build(routes, reg, true, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "http://10.10.0.12:30081") {
		t.Fatalf("hub mode must use hub HTTP upstream when effectiveMode unset:\n%s", s)
	}
	if strings.Contains(s, "https://10.10.0.12:30443") {
		t.Fatalf("hub mode must not use spoke HTTPS upstream:\n%s", s)
	}
}

func TestBuildHubRouteUsesHTTPUpstreamWhenEffectiveModeSite(t *testing.T) {
	routes := []fleetv1alpha1.FleetAppRoute{{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:    "abc-123",
			Cluster:       "virt02",
			Mode:          fleetv1alpha1.RouteModeHub,
			EffectiveMode: fleetv1alpha1.RouteModeSite,
			HubHost:       "myapp.mgmt.umkim.com",
		},
	}}
	reg := clusters.Registry{
		Clusters: map[string]clusters.Cluster{
			"virt02": {TlsPolicy: clusters.TLSPolicyACMECluster, UpstreamHost: "10.10.0.12", TraefikHubHTTPPort: 30081},
		},
	}
	out, err := Build(routes, reg, true, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "http://10.10.0.12:30081") {
		t.Fatalf("hub catalog route must force HTTP upstream:\n%s", s)
	}
}

func TestBuildHubRouteNoAuthByDefault(t *testing.T) {
	routes := []fleetv1alpha1.FleetAppRoute{{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:    "abc-123",
			Cluster:       "virt02",
			Mode:          fleetv1alpha1.RouteModeHub,
			EffectiveMode: fleetv1alpha1.RouteModeHub,
			HubHost:       "myapp.mgmt.umkim.com",
		},
	}}
	reg := clusters.Registry{
		Clusters: map[string]clusters.Cluster{
			"virt02": {DirectIngress: true, UpstreamHost: "10.10.0.3", TraefikHTTPPort: 30080},
		},
	}
	out, err := Build(routes, reg, false, authOpts())
	if err != nil {
		t.Fatal(err)
	}
	mids := hubRouterMiddlewares(fleetv1alpha1.RouteModeHub, false, false, authOpts(), false)
	for _, m := range mids {
		if m == "fleet-edge-authelia" {
			t.Fatalf("hub route without RequireAuth must not chain authelia: %v", mids)
		}
	}
	_ = out
}

func TestHubRouterMiddlewaresHubUsesAuthelia(t *testing.T) {
	opts := authOpts()
	mids := hubRouterMiddlewares(fleetv1alpha1.RouteModeHub, false, true, opts, false)
	if len(mids) != 2 || mids[1] != "fleet-edge-authelia" {
		t.Fatalf("hub-only route should chain authelia: %v", mids)
	}
}

func TestBuildDeterministicYAML(t *testing.T) {
	routes := []fleetv1alpha1.FleetAppRoute{
		{
			Spec: fleetv1alpha1.FleetAppRouteSpec{
				InstanceID:    "abc-123",
				Cluster:       "virt02",
				Mode:          fleetv1alpha1.RouteModeHub,
				EffectiveMode: fleetv1alpha1.RouteModeHub,
				HubHost:       "a.mgmt.umkim.com",
			},
		},
		{
			Spec: fleetv1alpha1.FleetAppRouteSpec{
				InstanceID:    "def-456",
				Cluster:       "virt02",
				Mode:          fleetv1alpha1.RouteModeHub,
				EffectiveMode: fleetv1alpha1.RouteModeHub,
				HubHost:       "b.mgmt.umkim.com",
			},
		},
	}
	reg := clusters.Registry{
		Clusters: map[string]clusters.Cluster{
			"virt02": {DirectIngress: true, UpstreamHost: "10.10.0.3", TraefikHTTPPort: 30080},
		},
	}
	opts := BuildOptions{
		AutheliaEnabled:        true,
		AutheliaForwardAuthURL: "http://authelia:9091/api/authz/forward-auth",
		AutheliaPortalHost:     "auth.mgmt.umkim.com",
		AutheliaServiceURL:     "http://authelia:9091",
	}
	first, err := Build(routes, reg, true, opts)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		out, err := Build(routes, reg, true, opts)
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != string(first) {
			t.Fatalf("build output changed on iteration %d", i)
		}
	}
}
