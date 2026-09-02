package manifests

import (
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
)

func TestHostsForRouteHub(t *testing.T) {
	route := fleetv1alpha1.FleetAppRoute{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			Mode:          fleetv1alpha1.RouteModeHub,
			EffectiveMode: fleetv1alpha1.RouteModeHub,
			HubHost:       "app.mgmt.umkim.com",
		},
	}
	hosts := HostsForRoute(route)
	if len(hosts) != 1 || hosts[0] != "app.mgmt.umkim.com" {
		t.Fatalf("hub mode hosts: %v", hosts)
	}
}

func TestSpokeHostsForRouteHub(t *testing.T) {
	route := fleetv1alpha1.FleetAppRoute{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			Mode:          fleetv1alpha1.RouteModeHub,
			EffectiveMode: fleetv1alpha1.RouteModeHub,
			HubHost:       "app.mgmt.umkim.com",
		},
	}
	got := SpokeHostsForRoute(route)
	if len(got) != 1 || got[0] != "app.mgmt.umkim.com" {
		t.Fatalf("hub mode spoke hosts: %v", got)
	}
}

func TestRenderManifestWorkHubNoAuth(t *testing.T) {
	route := fleetv1alpha1.FleetAppRoute{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:    "inst-hub",
			Cluster:       "virt01",
			Mode:          fleetv1alpha1.RouteModeHub,
			EffectiveMode: fleetv1alpha1.RouteModeHub,
			HubHost:       "app.mgmt.umkim.com",
			Service:       fleetv1alpha1.ServiceRef{Name: "app", Port: 80},
		},
	}
	hosts := SpokeHostsForRoute(route)
	if len(hosts) != 1 || hosts[0] != "app.mgmt.umkim.com" {
		t.Fatalf("hub mode spoke hosts: %v", hosts)
	}
	yaml, err := RenderManifestWork(route, hosts, "virt02", route.Spec.Service, SpokeIngressOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(yaml, "app.mgmt.umkim.com") {
		t.Fatalf("hub spoke ingress missing host: %s", yaml)
	}
}

func TestRenderManifestWorkSiteAuth(t *testing.T) {
	route := fleetv1alpha1.FleetAppRoute{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:    "inst-site",
			Cluster:       "virt01",
			Mode:          fleetv1alpha1.RouteModeSite,
			EffectiveMode: fleetv1alpha1.RouteModeSite,
			HubHost:       "app.mgmt.umkim.com",
			SiteHost:      "app.virt01.mgmt.umkim.com",
			RequireAuth:   true,
			Service:       fleetv1alpha1.ServiceRef{Name: "app", Port: 80},
		},
	}
	yaml, err := RenderManifestWork(route, []string{"app.virt01.mgmt.umkim.com"}, "virt01", route.Spec.Service, SpokeIngressOptions{AuthEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(yaml, siteAuthMiddleware) {
		t.Fatalf("site route with RequireAuth should reference authelia middleware: %s", yaml)
	}
}

func TestRenderManifestWorkSiteNoRequireAuth(t *testing.T) {
	route := fleetv1alpha1.FleetAppRoute{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:    "inst-site",
			Cluster:       "virt01",
			Mode:          fleetv1alpha1.RouteModeSite,
			EffectiveMode: fleetv1alpha1.RouteModeSite,
			SiteHost:      "app.virt01.mgmt.umkim.com",
			Service:       fleetv1alpha1.ServiceRef{Name: "app", Port: 80},
		},
	}
	yaml, err := RenderManifestWork(route, []string{"app.virt01.mgmt.umkim.com"}, "virt01", route.Spec.Service, SpokeIngressOptions{AuthEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(yaml, siteAuthMiddleware) {
		t.Fatalf("site route without RequireAuth must not attach authelia: %s", yaml)
	}
	if strings.Contains(yaml, "letsencrypt") {
		t.Fatalf("site route without ACME must not attach certresolver: %s", yaml)
	}
}

func TestRenderManifestWorkSiteACMENoAuth(t *testing.T) {
	route := fleetv1alpha1.FleetAppRoute{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:    "inst-site",
			Cluster:       "virt02",
			Mode:          fleetv1alpha1.RouteModeSite,
			EffectiveMode: fleetv1alpha1.RouteModeSite,
			SiteHost:      "demo.apps.umkim.com",
			Service:       fleetv1alpha1.ServiceRef{Name: "app", Port: 80},
		},
	}
	yaml, err := RenderManifestWork(route, []string{"demo.apps.umkim.com"}, "virt02", route.Spec.Service, SpokeIngressOptions{
		SpokeACME:     true,
		RedirectHTTPS: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(yaml, "kind: Ingress") != 1 {
		t.Fatalf("ACME site route should render exactly one HTTPS Ingress: %s", yaml)
	}
	if strings.Contains(yaml, traefikEntrypointsAnnotation+`: "web"`) {
		t.Fatalf("ACME site route must not create HTTP web ingress: %s", yaml)
	}
	if !strings.Contains(yaml, "letsencrypt") {
		t.Fatalf("ACME site route HTTPS ingress should use letsencrypt: %s", yaml)
	}
	if !strings.Contains(yaml, traefikEntrypointsAnnotation+": \"websecure\"") {
		t.Fatalf("ACME site route HTTPS ingress should use websecure: %s", yaml)
	}
	if strings.Contains(yaml, siteAuthMiddleware) {
		t.Fatalf("public ACME site route must not attach authelia: %s", yaml)
	}
	if !strings.Contains(yaml, "name: fleet-route-inst-site-tls") {
		t.Fatalf("expected separate TLS ingress: %s", yaml)
	}
}

func TestRenderManifestWorkSiteACMEStaging(t *testing.T) {
	route := fleetv1alpha1.FleetAppRoute{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:    "inst-site",
			Cluster:       "lab-single-0",
			Mode:          fleetv1alpha1.RouteModeSite,
			EffectiveMode: fleetv1alpha1.RouteModeSite,
			SiteHost:      "whoami.homelab.example.com",
			Service:       fleetv1alpha1.ServiceRef{Name: "app", Port: 80},
		},
	}
	yaml, err := RenderManifestWork(route, []string{"whoami.homelab.example.com"}, "lab-single-0", route.Spec.Service, SpokeIngressOptions{
		SpokeACME:     true,
		CertResolver:  "letsencrypt-staging",
		RedirectHTTPS: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(yaml, "letsencrypt-staging") {
		t.Fatalf("staging ACME ingress should use letsencrypt-staging resolver: %s", yaml)
	}
}

func TestRenderManifestWorkSiteACMEWithAuth(t *testing.T) {
	route := fleetv1alpha1.FleetAppRoute{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:    "inst-site",
			Cluster:       "virt02",
			Mode:          fleetv1alpha1.RouteModeSite,
			EffectiveMode: fleetv1alpha1.RouteModeSite,
			SiteHost:      "demo.apps.umkim.com",
			RequireAuth:   true,
			Service:       fleetv1alpha1.ServiceRef{Name: "app", Port: 80},
		},
	}
	yaml, err := RenderManifestWork(route, []string{"demo.apps.umkim.com"}, "virt02", route.Spec.Service, SpokeIngressOptions{
		AuthEnabled:   true,
		SpokeACME:     true,
		RedirectHTTPS: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(yaml, "kind: Ingress") != 1 {
		t.Fatalf("ACME site route should render exactly one HTTPS Ingress: %s", yaml)
	}
	if strings.Contains(yaml, traefikEntrypointsAnnotation+`: "web"`) {
		t.Fatalf("ACME site route must not create HTTP web ingress: %s", yaml)
	}
	if strings.Count(yaml, siteAuthMiddleware) != 1 {
		t.Fatalf("authelia should appear once on HTTPS ingress only: %s", yaml)
	}
	if !strings.Contains(yaml, "letsencrypt") {
		t.Fatalf("HTTPS ingress should use letsencrypt: %s", yaml)
	}
}

func TestHubHostsForRoute(t *testing.T) {
	hub := fleetv1alpha1.FleetAppRoute{Spec: fleetv1alpha1.FleetAppRouteSpec{
		Mode: fleetv1alpha1.RouteModeHub, HubHost: "h.example",
	}}
	if got := HubHostsForRoute(hub); len(got) != 1 || got[0] != "h.example" {
		t.Fatalf("hub hosts: %v", got)
	}
	site := fleetv1alpha1.FleetAppRoute{Spec: fleetv1alpha1.FleetAppRouteSpec{
		Mode: fleetv1alpha1.RouteModeSite, SiteHost: "s.example",
	}}
	if got := HubHostsForRoute(site); len(got) != 0 {
		t.Fatalf("site hub hosts should be empty: %v", got)
	}
}

func TestRenderManifestWorkPerClusterService(t *testing.T) {
	route := fleetv1alpha1.FleetAppRoute{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID: "inst-ms",
			Cluster:    "virt01",
			HubHost:    "app.mgmt.umkim.com",
			Service:    fleetv1alpha1.ServiceRef{Name: "app-primary", Port: 80},
			ClusterTargets: []fleetv1alpha1.ClusterTarget{
				{Cluster: "virt01", Role: "primary", Service: fleetv1alpha1.ServiceRef{Name: "app-primary", Port: 80}},
				{Cluster: "virt02", Role: "standby", Service: fleetv1alpha1.ServiceRef{Name: "app-standby", Port: 80}},
			},
		},
	}
	yamlPrimary, err := RenderManifestWork(route, []string{"app.mgmt.umkim.com"}, "virt01", fleetv1alpha1.ServiceRef{Name: "app-primary", Port: 80}, SpokeIngressOptions{})
	if err != nil {
		t.Fatal(err)
	}
	yamlStandby, err := RenderManifestWork(route, []string{"app.mgmt.umkim.com"}, "virt02", fleetv1alpha1.ServiceRef{Name: "app-standby", Port: 80}, SpokeIngressOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(yamlPrimary, "name: app-primary") {
		t.Fatalf("primary ingress should target app-primary: %s", yamlPrimary)
	}
	if strings.Contains(yamlPrimary, "app-standby") {
		t.Fatalf("primary ingress must not reference standby service: %s", yamlPrimary)
	}
	if !strings.Contains(yamlStandby, "name: app-standby") {
		t.Fatalf("standby ingress should target app-standby: %s", yamlStandby)
	}
}

func TestRenderManifestWork(t *testing.T) {
	route := fleetv1alpha1.FleetAppRoute{
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:    "inst-1",
			Cluster:       "virt02",
			Mode:          fleetv1alpha1.RouteModeSite,
			EffectiveMode: fleetv1alpha1.RouteModeSite,
			SiteHost:      "app.virt02.mgmt.umkim.com",
			Service:       fleetv1alpha1.ServiceRef{Name: "app", Port: 80, Namespace: "fleet-ingress"},
		},
	}
	yaml, err := RenderManifestWork(route, []string{"app.virt02.mgmt.umkim.com"}, "virt02", route.Spec.Service, SpokeIngressOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(yaml, "kind: ManifestWork") {
		t.Fatalf("missing ManifestWork: %s", yaml)
	}
	if !strings.Contains(yaml, "namespace: virt02") {
		t.Fatalf("missing cluster namespace: %s", yaml)
	}
	if !strings.Contains(yaml, "          rules:\n            - host:") {
		t.Fatalf("ingress rules not nested correctly: %s", yaml)
	}
}
