package routesync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestProviderToCluster(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"k8s-virt01", "virt01"},
		{"helm-virt02", "virt02"},
		{"olm-virt02-t2", "virt02-t2"},
		{"custom", "custom"},
	}
	for _, tc := range tests {
		if got := providerToCluster(tc.provider); got != tc.want {
			t.Fatalf("providerToCluster(%q) = %q, want %q", tc.provider, got, tc.want)
		}
	}
}

func TestInstanceResourceID(t *testing.T) {
	legacy := catalogInstance{ResourceID: "legacy-id"}
	if got := instanceResourceID(legacy); got != "legacy-id" {
		t.Fatalf("legacy resource_id = %q", got)
	}
	modern := catalogInstance{
		Spec: instSpec{ResourceIDs: []string{"from-spec"}},
	}
	if got := instanceResourceID(modern); got != "from-spec" {
		t.Fatalf("spec.resource_ids[0] = %q", got)
	}
	if got := instanceResourceID(catalogInstance{}); got != "" {
		t.Fatalf("empty = %q", got)
	}
}

func TestWorkloadServiceFromProvider(t *testing.T) {
	const resourceID = "c39c4fbf-0ccb-468e-bdad-8f521e4a02b2"
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1alpha1/providers":
			_ = json.NewEncoder(w).Encode(providersListResp{
				Providers: []struct {
					Name     string `json:"name"`
					Endpoint string `json:"endpoint"`
				}{
					{Name: "k8s-virt01", Endpoint: srv.URL + "/api/v1alpha1/containers"},
				},
			})
		case "/api/v1alpha1/containers/" + resourceID:
			_ = json.NewEncoder(w).Encode(containerWorkloadResp{
				Service: &struct {
					Name string `json:"name"`
				}{Name: "demo-webapp-fsnxf"},
				Spec: struct {
					Metadata struct {
						Name      string `json:"name"`
						Namespace string `json:"namespace"`
					} `json:"metadata"`
				}{
					Metadata: struct {
						Name      string `json:"name"`
						Namespace string `json:"namespace"`
					}{Name: "demo-webapp-fsnxf", Namespace: "default"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	syncer := &Syncer{
		Config: configForTest(srv.URL + "/api/v1alpha1"),
		HTTP:   srv.Client(),
	}
	ref, ok := syncer.workloadServiceFromProvider(context.Background(), "k8s-virt01", resourceID, "demo-webapp", "default")
	if !ok {
		t.Fatal("expected workload service")
	}
	if ref.Name != "demo-webapp-fsnxf" || ref.Namespace != "default" || ref.Port != 80 {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}

func TestPrimaryProviderFromPlacementTargets(t *testing.T) {
	const instanceID = "inst-abc"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/api/v1alpha1/catalog-item-instances/" + instanceID + "/placement-targets"
		if r.URL.Path != want {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(placementTargetsResp{
			Targets: []placementTarget{
				{Provider: "k8s-virt02", Role: "standby", SprmID: "sprm-standby"},
				{Provider: "k8s-virt01", Role: "primary", SprmID: "sprm-primary"},
			},
		})
	}))
	defer srv.Close()

	syncer := &Syncer{
		Config: configForTest(srv.URL + "/api/v1alpha1"),
		HTTP:   srv.Client(),
	}
	got := syncer.primaryProviderFromPlacementTargets(context.Background(), instanceID)
	if got != "k8s-virt01" {
		t.Fatalf("primaryProviderFromPlacementTargets() = %q, want k8s-virt01", got)
	}
}

func TestPrimaryFromPlacementTargets(t *testing.T) {
	const instanceID = "inst-abc"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/api/v1alpha1/catalog-item-instances/" + instanceID + "/placement-targets"
		if r.URL.Path != want {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(placementTargetsResp{
			Targets: []placementTarget{
				{Provider: "k8s-virt02", Role: "standby", SprmID: "sprm-standby"},
				{Provider: "k8s-virt01", Role: "primary", SprmID: "sprm-primary"},
			},
		})
	}))
	defer srv.Close()

	syncer := &Syncer{
		Config: configForTest(srv.URL + "/api/v1alpha1"),
		HTTP:   srv.Client(),
	}
	got := syncer.primaryFromPlacementTargets(context.Background(), instanceID)
	if got != "virt01" {
		t.Fatalf("primaryFromPlacementTargets() = %q, want virt01", got)
	}
}

func TestPrimaryFromPlacementTargetsFirstFallback(t *testing.T) {
	const instanceID = "inst-fallback"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(placementTargetsResp{
			Targets: []placementTarget{
				{Provider: "helm-virt02", Role: ""},
			},
		})
	}))
	defer srv.Close()

	syncer := &Syncer{
		Config: configForTest(srv.URL + "/api/v1alpha1"),
		HTTP:   srv.Client(),
	}
	got := syncer.primaryFromPlacementTargets(context.Background(), instanceID)
	if got != "virt02" {
		t.Fatalf("primaryFromPlacementTargets() = %q, want virt02", got)
	}
}

func TestPrimaryFromPlacementTargetsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(placementTargetsResp{})
	}))
	defer srv.Close()

	syncer := &Syncer{
		Config: configForTest(srv.URL + "/api/v1alpha1"),
		HTTP:   srv.Client(),
	}
	if got := syncer.primaryFromPlacementTargets(context.Background(), "missing"); got != "" {
		t.Fatalf("expected empty cluster, got %q", got)
	}
}

func configForTest(catalogURL string) config.Config {
	return config.Config{CatalogAPIURL: catalogURL}
}

func TestResolveTargetServices(t *testing.T) {
	const instanceID = "inst-ms"
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1alpha1/catalog-item-instances/"+instanceID+"/placement-targets":
			_ = json.NewEncoder(w).Encode(placementTargetsResp{
				Targets: []placementTarget{
					{Provider: "k8s-virt01", Role: "primary", SprmID: "sprm-primary"},
					{Provider: "k8s-virt02", Role: "standby", SprmID: "sprm-standby"},
				},
			})
		case r.URL.Path == "/api/v1alpha1/providers":
			_ = json.NewEncoder(w).Encode(providersListResp{
				Providers: []struct {
					Name     string `json:"name"`
					Endpoint string `json:"endpoint"`
				}{
					{Name: "k8s-virt01", Endpoint: srv.URL + "/api/v1alpha1/containers"},
					{Name: "k8s-virt02", Endpoint: srv.URL + "/api/v1alpha1/containers"},
				},
			})
		case r.URL.Path == "/api/v1alpha1/containers/sprm-primary":
			_ = json.NewEncoder(w).Encode(containerWorkloadResp{
				Service: &struct {
					Name string `json:"name"`
				}{Name: "svc-primary"},
			})
		case r.URL.Path == "/api/v1alpha1/containers/sprm-standby":
			_ = json.NewEncoder(w).Encode(containerWorkloadResp{
				Service: &struct {
					Name string `json:"name"`
				}{Name: "svc-standby"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	syncer := &Syncer{
		Config: configForTest(srv.URL + "/api/v1alpha1"),
		HTTP:   srv.Client(),
	}
	got := syncer.resolveTargetServices(context.Background(), instanceID, "resource-base", "virt01", "virt02", "app", "default", 80)
	if len(got) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(got))
	}
	if got[0].Service.Name != "svc-primary" || got[0].SprmID != "sprm-primary" {
		t.Fatalf("unexpected primary: %+v", got[0])
	}
	if got[1].Service.Name != "svc-standby" || got[1].SprmID != "sprm-standby" {
		t.Fatalf("unexpected standby: %+v", got[1])
	}
}

func TestSyncInstanceUpdatesCatalogSiteHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	const (
		ns  = "fleet-system"
		uid = "abcdefghijkl"
	)
	existing := &fleetv1alpha1.FleetAppRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route-" + uid, Namespace: ns},
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID:    uid,
			Mode:          fleetv1alpha1.RouteModeSite,
			EffectiveMode: fleetv1alpha1.RouteModeHub,
			Cluster:       "lab",
			HubHost:       "demo.example.com",
			SiteHost:      "site-a.example.com",
			HealthPath:    "/",
			Service:       fleetv1alpha1.ServiceRef{Namespace: "default", Name: "demo", Port: 80},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	syncer := &Syncer{
		Client: c,
		Config: config.Config{
			CatalogAPIURL:   srv.URL + "/api/v1alpha1",
			RouteNamespace:  ns,
			FleetEdgeDomain: "example.com",
		},
		HTTP: srv.Client(),
	}
	inst := catalogInstance{
		UID: uid,
		Spec: instSpec{
			CatalogItemID: "item",
			ResourceIDs:   []string{"res-1"},
			UserValues: []userValue{
				{Path: "routing.mode", Value: "site"},
				{Path: "routing.cluster", Value: "lab"},
				{Path: "routing.site_host", Value: "site-b.example.com"},
				{Path: "routing.hub_host", Value: "demo.example.com"},
				{Path: "metadata.name", Value: "demo"},
			},
		},
	}
	if err := syncer.syncInstance(context.Background(), inst); err != nil {
		t.Fatal(err)
	}
	var got fleetv1alpha1.FleetAppRoute
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.SiteHost != "site-b.example.com" {
		t.Fatalf("site host = %q, want site-b.example.com", got.Spec.SiteHost)
	}
	if got.Spec.EffectiveMode != fleetv1alpha1.RouteModeHub {
		t.Fatalf("effective mode = %q, want hub", got.Spec.EffectiveMode)
	}

	inst.Spec.UserValues[2].Value = ""
	if err := syncer.syncInstance(context.Background(), inst); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.SiteHost != "site-b.example.com" {
		t.Fatalf("empty catalog host replaced site host with %q", got.Spec.SiteHost)
	}
	if got.Spec.EffectiveMode != fleetv1alpha1.RouteModeHub {
		t.Fatalf("effective mode = %q, want hub", got.Spec.EffectiveMode)
	}
}
