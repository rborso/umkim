package reconcile

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
)

func TestEffectiveModeHubOnlyCluster(t *testing.T) {
	reg := clusters.Registry{
		Clusters: map[string]clusters.Cluster{
			"virt01-k3s": {DirectIngress: false, UpstreamHost: "10.0.0.1", TraefikHTTPPort: 30080},
		},
	}
	mode, msg := effectiveMode(fleetv1alpha1.RouteModeDual, "virt01-k3s", reg)
	if mode != fleetv1alpha1.RouteModeHub {
		t.Fatalf("expected hub, got %q (%s)", mode, msg)
	}
}

func TestEffectiveModeDirectIngress(t *testing.T) {
	reg := clusters.Registry{
		Clusters: map[string]clusters.Cluster{
			"virt02": {DirectIngress: true, UpstreamHost: "10.0.0.2", TraefikHTTPPort: 30080},
		},
	}
	mode, _ := effectiveMode(fleetv1alpha1.RouteModeDual, "virt02", reg)
	if mode != fleetv1alpha1.RouteModeDual {
		t.Fatalf("expected dual, got %q", mode)
	}
}

func TestEffectiveModeReverseProxy(t *testing.T) {
	reg := clusters.Registry{
		Clusters: map[string]clusters.Cluster{
			"virt01": {ExposureMode: "reverse_proxy", TlsPolicy: "external", UpstreamHost: "10.0.0.1", TraefikHTTPPort: 30080},
		},
	}
	mode, _ := effectiveMode(fleetv1alpha1.RouteModeSite, "virt01", reg)
	if mode != fleetv1alpha1.RouteModeSite {
		t.Fatalf("expected site, got %q", mode)
	}
}

func TestEffectiveModeNone(t *testing.T) {
	reg := clusters.Registry{
		Clusters: map[string]clusters.Cluster{
			"virt02": {DirectIngress: true, UpstreamHost: "10.0.0.2", TraefikHTTPPort: 30080},
		},
	}
	mode, msg := effectiveMode(fleetv1alpha1.RouteModeNone, "virt02", reg)
	if mode != fleetv1alpha1.RouteModeNone {
		t.Fatalf("expected none, got %q (%s)", mode, msg)
	}
}

func TestDefaultSiteHost(t *testing.T) {
	got := defaultSiteHost("myapp.mgmt.umkim.com", "virt02", "mgmt.umkim.com")
	want := "myapp.virt02.mgmt.umkim.com"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
