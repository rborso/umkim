package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	fleetedge "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testIngressScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := fleetedge.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func testIngressServer(t *testing.T, objs ...client.Object) *Server {
	t.Helper()
	return &Server{
		Client: fake.NewClientBuilder().WithScheme(testIngressScheme(t)).WithObjects(objs...).Build(),
		Cfg: config.Config{
			Namespace:               "fleet-console",
			EdgeClustersCMNamespace: "fleet-system",
			EdgeClustersCMName:      "fleet-edge-clusters",
			EdgeRoutesNamespace:     "fleet-system",
			ManagementClusterName:   "umkim-hub",
			HubEdgeDomain:           "demo.example.test",
			RequireIdentity:         false,
		},
	}
}

func pfRow(id, zone, address string) fleetv1alpha1.FleetSiteIngressSpec {
	return fleetv1alpha1.FleetSiteIngressSpec{
		ID: id, Method: fleetv1alpha1.IngressMethodPortForward, Zone: zone, Address: address,
	}
}

func TestValidateSiteIngressRowsRejectsDuplicateWildcard(t *testing.T) {
	edge := fleetv1alpha1.FleetSiteEdgeSpec{
		WanEgress: &fleetv1alpha1.FleetSiteWanEgressSpec{Enabled: true},
	}
	rows := []fleetv1alpha1.FleetSiteIngressSpec{
		pfRow("homelab-pf", "homelab.example.test", "172.16.10.100"),
		pfRow("homelab-ha-pf", "homelab.example.test", "172.16.10.100"),
	}
	err := validateSiteIngressRows(edge, rows)
	if err == nil {
		t.Fatal("want duplicate wildcard error, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("duplicates wildcard")) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSiteIngressRowsAllowsDistinctWildcards(t *testing.T) {
	edge := fleetv1alpha1.FleetSiteEdgeSpec{
		WanEgress: &fleetv1alpha1.FleetSiteWanEgressSpec{Enabled: true},
	}
	rows := []fleetv1alpha1.FleetSiteIngressSpec{
		pfRow("homelab-pf", "homelab.example.test", "172.16.10.100"),
		pfRow("homelab-gpu-pf", "homelab.example.test", "172.16.10.141"),
	}
	rows[1].Wildcard = "*.gpu.homelab.example.test"
	if err := validateSiteIngressRows(edge, rows); err != nil {
		t.Fatalf("distinct wildcards must be accepted: %v", err)
	}
}

func TestValidateSiteIngressRowsStillRejectsDuplicateID(t *testing.T) {
	edge := fleetv1alpha1.FleetSiteEdgeSpec{}
	rows := []fleetv1alpha1.FleetSiteIngressSpec{
		pfRow("same", "a.example.test", "10.0.0.1"),
		pfRow("same", "b.example.test", "10.0.0.2"),
	}
	if err := validateSiteIngressRows(edge, rows); err == nil {
		t.Fatal("want duplicate id error")
	}
}

func ingressTestSite() *fleetv1alpha1.FleetSite {
	return &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName: "homelab",
			SiteId:   "homelab",
			Edge: fleetv1alpha1.FleetSiteEdgeSpec{
				HubGateway:  true,
				HubZone:     "demo.example.test",
				HubWildcard: "*.demo.example.test",
				Ingress: []fleetv1alpha1.FleetSiteIngressSpec{
					pfRow("homelab-pf", "homelab.example.test", "172.16.10.100"),
				},
			},
		},
	}
}

func ingressTestJoin() *fleetv1alpha1.FleetClusterJoin {
	return &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "lab-single-0", Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName: "lab-single-0",
			SiteId:      "homelab",
			Edge:        fleetv1alpha1.FleetClusterJoinEdgeSpec{IngressId: "homelab-pf"},
		},
	}
}

func TestValidateAppRouteHostsSite(t *testing.T) {
	s := testIngressServer(t, ingressTestSite(), ingressTestJoin())
	ctx := context.Background()

	inWildcard := []fleetv1alpha1.FleetAppRouteIntent{
		{Mode: fleetv1alpha1.AppRouteModeSite, SiteHost: "gitea.homelab.example.test"},
	}
	if msg := s.validateAppRouteHosts(ctx, "lab-single-0", inWildcard); msg != "" {
		t.Fatalf("in-wildcard host rejected: %s", msg)
	}

	outside := []fleetv1alpha1.FleetAppRouteIntent{
		{Mode: fleetv1alpha1.AppRouteModeSite, SiteHost: "gitea.elsewhere.example.test"},
	}
	msg := s.validateAppRouteHosts(ctx, "lab-single-0", outside)
	if msg == "" {
		t.Fatal("host outside the ingress wildcard must be rejected")
	}
	if !bytes.Contains([]byte(msg), []byte("wildcard")) {
		t.Fatalf("unexpected message: %s", msg)
	}
}

func TestValidateAppRouteHostsHub(t *testing.T) {
	s := testIngressServer(t, ingressTestSite(), ingressTestJoin())
	ctx := context.Background()

	ok := []fleetv1alpha1.FleetAppRouteIntent{
		{Mode: fleetv1alpha1.AppRouteModeHub, HubHost: "gitea.demo.example.test"},
	}
	if msg := s.validateAppRouteHosts(ctx, "lab-single-0", ok); msg != "" {
		t.Fatalf("hub host under the hub wildcard rejected: %s", msg)
	}
	bad := []fleetv1alpha1.FleetAppRouteIntent{
		{Mode: fleetv1alpha1.AppRouteModeHub, HubHost: "gitea.homelab.example.test"},
	}
	if msg := s.validateAppRouteHosts(ctx, "lab-single-0", bad); msg == "" {
		t.Fatal("hub host outside the site hub wildcard must be rejected")
	}
}

func TestValidateAppRouteHostsNoIngressBinding(t *testing.T) {
	join := ingressTestJoin()
	join.Spec.Edge.IngressId = ""
	s := testIngressServer(t, ingressTestSite(), join)
	// No explicit binding and the site has two ingress rows, so AutoIngressID is
	// empty: the site host cannot be checked and must not be rejected here.
	extra := pfRow("homelab-gpu-pf", "homelab.example.test", "172.16.10.141")
	fs := ingressTestSite()
	fs.Spec.Edge.Ingress = append(fs.Spec.Edge.Ingress, extra)
	s = testIngressServer(t, fs, join)
	msg := s.validateAppRouteHosts(context.Background(), "lab-single-0", []fleetv1alpha1.FleetAppRouteIntent{
		{Mode: fleetv1alpha1.AppRouteModeSite, SiteHost: "anything.example.test"},
	})
	if msg != "" {
		t.Fatalf("unresolvable binding must not reject the host: %s", msg)
	}
}

func TestVerifySiteIngressRejectsDuplicateWildcardAndReportsBinding(t *testing.T) {
	fs := ingressTestSite()
	fs.Spec.Edge.Ingress = append(fs.Spec.Edge.Ingress, pfRow("homelab-ha-pf", "homelab.example.test", "172.16.10.100"))
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-edge-clusters", Namespace: "fleet-system"},
		Data:       map[string]string{"clusters.yaml": "clusters:\n  lab-single-0:\n    site: homelab\n    ingress_id: homelab-pf\n    method: port_forward\n    upstream_host: 10.10.0.14\n    traefik_http_port: 30080\n"},
	}
	s := testIngressServer(t, fs, cm)

	oldLookup, oldDial := ingressVerifyLookupHost, ingressVerifyDial
	defer func() { ingressVerifyLookupHost, ingressVerifyDial = oldLookup, oldDial }()
	ingressVerifyLookupHost = func(_ context.Context, host string) ([]string, error) {
		return []string{"159.195.137.119"}, nil
	}
	ingressVerifyDial = func(_ context.Context, address string) error { return nil }

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites/homelab/ingress/verify", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	s.verifySiteIngress(context.Background(), rec, req, "homelab")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp ingressVerifyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.OK {
		t.Fatal("response must not be OK when two entries share a wildcard")
	}
	if len(resp.Rows) != 2 {
		t.Fatalf("rows=%d want 2", len(resp.Rows))
	}
	bound := resp.Rows[0]
	if len(bound.AssignedClusters) != 1 || bound.AssignedClusters[0] != "lab-single-0" {
		t.Fatalf("assigned=%v want [lab-single-0]", bound.AssignedClusters)
	}
	if !checkStatus(bound.Checks, "wildcard", ingressVerifyFail) {
		t.Fatalf("bound row must fail the wildcard check: %+v", bound.Checks)
	}
	if !checkStatus(bound.Checks, "binding", ingressVerifyOK) {
		t.Fatalf("bound row must pass the binding check: %+v", bound.Checks)
	}
	unbound := resp.Rows[1]
	if !checkStatus(unbound.Checks, "binding", ingressVerifyWarn) {
		t.Fatalf("unbound row must warn on binding: %+v", unbound.Checks)
	}
	if !checkStatus(unbound.Checks, "upstream", ingressVerifySkip) {
		t.Fatalf("unbound row must skip the upstream check: %+v", unbound.Checks)
	}
}

func TestVerifySiteIngressSingleID(t *testing.T) {
	fs := ingressTestSite()
	fs.Spec.Edge.Ingress = append(fs.Spec.Edge.Ingress, pfRow("homelab-ha-pf", "homelab.example.test", "172.16.10.100"))
	s := testIngressServer(t, fs)
	oldLookup, oldDial := ingressVerifyLookupHost, ingressVerifyDial
	defer func() { ingressVerifyLookupHost, ingressVerifyDial = oldLookup, oldDial }()
	ingressVerifyLookupHost = func(_ context.Context, _ string) ([]string, error) { return nil, nil }
	ingressVerifyDial = func(_ context.Context, _ string) error { return nil }

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"id":"homelab-ha-pf"}`))
	rec := httptest.NewRecorder()
	s.verifySiteIngress(context.Background(), rec, req, "homelab")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp ingressVerifyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Rows) != 1 || resp.Rows[0].ID != "homelab-ha-pf" {
		t.Fatalf("rows=%+v want only homelab-ha-pf", resp.Rows)
	}
}

func TestVerifySiteIngressUnknownID(t *testing.T) {
	s := testIngressServer(t, ingressTestSite())
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"id":"nope"}`))
	rec := httptest.NewRecorder()
	s.verifySiteIngress(context.Background(), rec, req, "homelab")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func checkStatus(checks []ingressVerifyCheck, name, status string) bool {
	for _, c := range checks {
		if c.Name == name {
			return c.Status == status
		}
	}
	return false
}
