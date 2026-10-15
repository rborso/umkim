package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testAppHTTPScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func testAppServer(t *testing.T, objs ...client.Object) *Server {
	t.Helper()
	scheme := testAppHTTPScheme(t)
	return &Server{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
		Cfg: config.Config{
			Namespace:             "fleet-console",
			HubWireguardIP:        "10.10.0.1",
			ManagementClusterName: "umkim-hub",
			HelmOCIPrefixes:       "oci://10.10.0.1:30443/charts",
			HubEdgeDomain:         "demo.example.test",
			RequireIdentity:       false,
		},
	}
}

func readyJoin(name string) *fleetv1alpha1.FleetClusterJoin {
	return &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "fleet-console"},
		Spec:       fleetv1alpha1.FleetClusterJoinSpec{ClusterName: name, SiteId: "homelab"},
		Status:     fleetv1alpha1.FleetClusterJoinStatus{Phase: fleetv1alpha1.JoinPhaseReady},
	}
}

func TestCreateAppRequiresCluster(t *testing.T) {
	s := testAppServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/apps", bytes.NewBufferString(`{"chart":"whoami"}`))
	rec := httptest.NewRecorder()
	s.handleApps(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateAppRejectsNonReadyCluster(t *testing.T) {
	pending := readyJoin("lab-single-0")
	pending.Status.Phase = fleetv1alpha1.JoinPhaseValidated
	s := testAppServer(t, pending)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/apps", bytes.NewBufferString(`{
		"clusterName":"lab-single-0","chart":"whoami"
	}`))
	rec := httptest.NewRecorder()
	s.handleApps(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateAppRejectsHubCluster(t *testing.T) {
	s := testAppServer(t, readyJoin("umkim-hub"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/apps", bytes.NewBufferString(`{
		"clusterName":"umkim-hub","chart":"whoami"
	}`))
	rec := httptest.NewRecorder()
	s.handleApps(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateAppRejectsOffPrefixChart(t *testing.T) {
	s := testAppServer(t, readyJoin("lab-single-0"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/apps", bytes.NewBufferString(`{
		"clusterName":"lab-single-0","chart":"oci://evil.example/charts/x"
	}`))
	rec := httptest.NewRecorder()
	s.handleApps(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateAppExpandsShortChartName(t *testing.T) {
	s := testAppServer(t, readyJoin("lab-single-0"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/apps", bytes.NewBufferString(`{
		"clusterName":"lab-single-0","chart":"whoami","version":"0.1.0","releaseName":"whoami"
	}`))
	rec := httptest.NewRecorder()
	s.handleApps(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp appResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Chart != "oci://10.10.0.1:30443/charts/whoami" {
		t.Fatalf("chart=%q", resp.Chart)
	}
	if resp.ClusterName != "lab-single-0" || resp.ReleaseName != "whoami" {
		t.Fatalf("resp=%+v", resp)
	}
	var stored fleetv1alpha1.FleetApp
	if err := s.Client.Get(req.Context(), client.ObjectKey{Namespace: "fleet-console", Name: "whoami"}, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Spec.ClusterName != "lab-single-0" {
		t.Fatalf("stored cluster=%q", stored.Spec.ClusterName)
	}
}

func TestPatchAppUpdatesVersion(t *testing.T) {
	app := &fleetv1alpha1.FleetApp{
		ObjectMeta: metav1.ObjectMeta{Name: "whoami", Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetAppSpec{
			ClusterName: "lab-single-0",
			Chart:       "oci://10.10.0.1:30443/charts/whoami",
			Version:     "0.1.0",
			ReleaseName: "whoami",
		},
		Status: fleetv1alpha1.FleetAppStatus{Phase: fleetv1alpha1.AppPhaseReady},
	}
	s := testAppServer(t, app)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1alpha1/apps/whoami", bytes.NewBufferString(`{"version":"0.2.0"}`))
	rec := httptest.NewRecorder()
	s.handleAppByName(rec, req, "whoami")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stored fleetv1alpha1.FleetApp
	if err := s.Client.Get(req.Context(), client.ObjectKey{Namespace: "fleet-console", Name: "whoami"}, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Spec.Version != "0.2.0" {
		t.Fatalf("version=%q", stored.Spec.Version)
	}
}

func TestPatchAppUpdatesRouteGroups(t *testing.T) {
	app := &fleetv1alpha1.FleetApp{
		ObjectMeta: metav1.ObjectMeta{Name: "whoami", Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetAppSpec{
			ClusterName: "lab-single-0",
			Chart:       "oci://10.10.0.1:30443/charts/whoami",
			ReleaseName: "whoami",
			Route: fleetv1alpha1.FleetAppRouteIntent{
				Mode:     fleetv1alpha1.AppRouteModeSite,
				SiteHost: "whoami.example.com",
			},
		},
		Status: fleetv1alpha1.FleetAppStatus{Phase: fleetv1alpha1.AppPhaseReady},
	}
	s := testAppServer(t, app)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1alpha1/apps/whoami",
		bytes.NewBufferString(`{"route":{"requireAuth":true,"requireAuthGroups":["labtenant"]}}`))
	rec := httptest.NewRecorder()
	s.handleAppByName(rec, req, "whoami")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stored fleetv1alpha1.FleetApp
	if err := s.Client.Get(req.Context(), client.ObjectKey{Namespace: "fleet-console", Name: "whoami"}, &stored); err != nil {
		t.Fatal(err)
	}
	// These were silently dropped, so `app update --require-auth-groups` changed
	// nothing while reporting success.
	if !stored.Spec.Route.RequireAuth {
		t.Fatal("requireAuth was not applied")
	}
	if len(stored.Spec.Route.RequireAuthGroups) != 1 || stored.Spec.Route.RequireAuthGroups[0] != "labtenant" {
		t.Fatalf("requireAuthGroups=%v", stored.Spec.Route.RequireAuthGroups)
	}
	// The rest of the route is untouched by a partial update.
	if stored.Spec.Route.SiteHost != "whoami.example.com" || stored.Spec.Route.Mode != fleetv1alpha1.AppRouteModeSite {
		t.Fatalf("route replaced: %+v", stored.Spec.Route)
	}
}

func TestPatchAppRejectsInstalling(t *testing.T) {
	app := &fleetv1alpha1.FleetApp{
		ObjectMeta: metav1.ObjectMeta{Name: "whoami", Namespace: "fleet-console"},
		Spec:       fleetv1alpha1.FleetAppSpec{ClusterName: "lab-single-0", Chart: "whoami"},
		Status:     fleetv1alpha1.FleetAppStatus{Phase: fleetv1alpha1.AppPhaseInstalling},
	}
	s := testAppServer(t, app)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1alpha1/apps/whoami", bytes.NewBufferString(`{"version":"0.2.0"}`))
	rec := httptest.NewRecorder()
	s.handleAppByName(rec, req, "whoami")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListAppsIncludesDefaults(t *testing.T) {
	s := testAppServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/apps", nil)
	rec := httptest.NewRecorder()
	s.handleApps(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp appsListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Defaults.ChartPrefix != "oci://10.10.0.1:30443/charts" {
		t.Fatalf("prefix=%q", resp.Defaults.ChartPrefix)
	}
	if resp.Apps == nil {
		t.Fatal("apps should be empty list not null")
	}
}
