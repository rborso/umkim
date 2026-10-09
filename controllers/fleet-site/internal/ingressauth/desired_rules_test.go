package ingressauth

import (
	"context"
	"strings"
	"testing"

	fleetedge "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLoadRouteAccessRules(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := fleetedge.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	site := &fleetedge.FleetAppRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "sonarr", Namespace: "fleet-system"},
		Spec: fleetedge.FleetAppRouteSpec{
			Cluster: "lab-ha-0", Mode: fleetedge.RouteModeSite, SiteHost: "sonarr.example.com",
			RequireAuth: true, RequireAuthGroups: []string{"tenant-b", "tenant-a", "tenant-a"},
		},
	}
	hub := &fleetedge.FleetAppRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "hubapp", Namespace: "fleet-system"},
		Spec: fleetedge.FleetAppRouteSpec{
			Cluster: "lab-ha-0", Mode: fleetedge.RouteModeHub, HubHost: "hubapp.example.com",
			RequireAuth: true, RequireAuthGroups: []string{"ops"},
		},
	}
	noGroups := &fleetedge.FleetAppRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "open", Namespace: "fleet-system"},
		Spec: fleetedge.FleetAppRouteSpec{
			Cluster: "lab-ha-0", Mode: fleetedge.RouteModeSite, SiteHost: "open.example.com",
			RequireAuth: true,
		},
	}
	notRequired := &fleetedge.FleetAppRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "disabled", Namespace: "fleet-system"},
		Spec: fleetedge.FleetAppRouteSpec{
			Cluster: "lab-ha-0", Mode: fleetedge.RouteModeSite, SiteHost: "disabled.example.com",
			RequireAuth: false, RequireAuthGroups: []string{"ops"},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(site, hub, noGroups, notRequired).Build()
	res, err := loadRouteAccessRules(context.Background(), c, config.Config{EdgeRoutesNamespace: "fleet-system"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.hub) != 1 || res.hub[0].Domain != "hubapp.example.com" || strings.Join(res.hub[0].Groups, ",") != "ops" {
		t.Fatalf("hub rules = %+v", res.hub)
	}
	got := res.byCluster["lab-ha-0"]
	if len(got) != 1 {
		t.Fatalf("cluster rules = %+v", got)
	}
	if got[0].Domain != "sonarr.example.com" || strings.Join(got[0].Groups, ",") != "tenant-a,tenant-b" {
		t.Fatalf("cluster rule = %+v", got[0])
	}
}

func TestBuildInventoryYAMLAccessRules(t *testing.T) {
	desired := DesiredState{
		EnabledClusters: []string{"lab-ha-0"},
		AccessRules:     []RouteAuthRule{{Domain: "hubapp.example.com", Groups: []string{"ops"}}},
		ClusterConfigs: map[string]ClusterAuthConfig{
			"lab-ha-0": {
				PortalHost:   "auth.example.com",
				CookieDomain: "example.com",
				AccessRules:  []RouteAuthRule{{Domain: "sonarr.example.com", Groups: []string{"tenant-a"}}},
			},
		},
	}
	out := BuildInventoryYAML(config.Config{}, desired, &fleetv1alpha1.FleetIngressAuth{}, nil)
	for _, want := range []string{
		"fleet_ingress_auth_access_rules:",
		"  - domain: \"hubapp.example.com\"",
		"      - \"ops\"",
		"    access_rules:",
		"      - domain: \"sonarr.example.com\"",
		"          - \"tenant-a\"",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("inventory missing %q\n%s", want, out)
		}
	}
}
