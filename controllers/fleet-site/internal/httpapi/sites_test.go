package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testSiteHTTPScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func TestNormalizeSiteIDOnCreate(t *testing.T) {
	fs := &fleetv1alpha1.FleetSite{
		Spec: fleetv1alpha1.FleetSiteSpec{SiteName: "homelab", SiteId: "h"},
	}
	normalizeSiteIDOnCreate(fs)
	if fs.Spec.SiteId != "homelab" {
		t.Fatalf("siteId=%q want homelab", fs.Spec.SiteId)
	}
}

func TestNormalizeSiteIDOnCreateSkipsMultiCharPrefix(t *testing.T) {
	fs := &fleetv1alpha1.FleetSite{
		Spec: fleetv1alpha1.FleetSiteSpec{SiteName: "homelab", SiteId: "ho"},
	}
	normalizeSiteIDOnCreate(fs)
	if fs.Spec.SiteId != "ho" {
		t.Fatalf("siteId=%q want ho", fs.Spec.SiteId)
	}
}

func TestDeleteRegistrySite(t *testing.T) {
	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "homelab",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
		},
	}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site).Build(),
		Cfg:    config.Config{Namespace: ns, ManagementSiteID: "umkim"},
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1alpha1/sites/homelab", nil)
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var gone fleetv1alpha1.FleetSite
	err := s.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "homelab"}, &gone)
	if err == nil {
		t.Fatal("expected site deleted")
	}
}

func TestPatchSiteMetadata(t *testing.T) {
	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "homelab",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
		},
	}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site).Build(),
		Cfg:    config.Config{Namespace: ns, ManagementSiteID: "umkim", RequireIdentity: false},
	}
	body, _ := json.Marshal(map[string]string{
		"jumphost":       "jump@203.0.113.1:22",
		"region":         "homelab",
		"failureDomain":  "rack-a",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1alpha1/sites/homelab", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handlePatchSite(context.Background(), rec, req, "homelab")

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var updated fleetv1alpha1.FleetSite
	if err := s.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "homelab"}, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Spec.Jumphost != "jump@203.0.113.1:22" {
		t.Fatalf("jumphost=%q", updated.Spec.Jumphost)
	}
	if updated.Spec.Region != "homelab" {
		t.Fatalf("region=%q", updated.Spec.Region)
	}
	if updated.Spec.FailureDomain != "rack-a" {
		t.Fatalf("failureDomain=%q", updated.Spec.FailureDomain)
	}
}

func TestPatchSiteInvalidJumphost(t *testing.T) {
	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "homelab",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
		},
	}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}
	body, _ := json.Marshal(map[string]string{"jumphost": "bad"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1alpha1/sites/homelab", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handlePatchSite(context.Background(), rec, req, "homelab")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteManagementSiteForbidden(t *testing.T) {
	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "umkim",
			Namespace: ns,
			Annotations: map[string]string{
				"fleet.umkim.com/management-site": "true",
			},
		},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "umkim",
			SiteId:           "umkim",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
		},
	}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site).Build(),
		Cfg:    config.Config{Namespace: ns, ManagementSiteID: "umkim"},
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1alpha1/sites/umkim", nil)
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", rec.Code, rec.Body.String())
	}
}

func TestPostSiteNormalizesLatchedSiteID(t *testing.T) {
	ns := "fleet-console"
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).Build(),
		Cfg:    config.Config{Namespace: ns},
	}
	body := map[string]interface{}{
		"apiVersion": "fleet.umkim.com/v1alpha1",
		"kind":       "FleetSite",
		"metadata":   map[string]string{"name": "homelab"},
		"spec": map[string]interface{}{
			"siteName":         "homelab",
			"siteId":           "h",
			"provisioningMode": "registry",
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	s.handleSites(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "homelab"}, &fs); err != nil {
		t.Fatal(err)
	}
	if fs.Spec.SiteId != "homelab" {
		t.Fatalf("siteId=%q want homelab", fs.Spec.SiteId)
	}
}

func TestPatchSiteIngressSyncsEdgeClusters(t *testing.T) {
	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "homelab",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
			Edge: fleetv1alpha1.FleetSiteEdgeSpec{
				WanEgress: &fleetv1alpha1.FleetSiteWanEgressSpec{Enabled: true},
				Ingress: []fleetv1alpha1.FleetSiteIngressSpec{{
					ID: "homelab-pf", Method: fleetv1alpha1.IngressMethodPortForward,
					Address: "203.0.113.50", Zone: "homelab.example.com",
				}},
			},
		},
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-edge-clusters", Namespace: "fleet-system"},
		Data: map[string]string{
			"clusters.yaml": `clusters:
  lab-single-0:
    site: homelab
    direct_ingress: true
    ingress_id: homelab-pf
    method: port_forward
    edge_wan_ip: 203.0.113.50
    upstream_host: 10.10.0.12
    traefik_http_port: 30080
`,
		},
	}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site, cm).Build(),
		Cfg: config.Config{
			Namespace:               ns,
			ManagementSiteID:        "umkim",
			RequireIdentity:         false,
			EdgeClustersCMNamespace: "fleet-system",
			EdgeClustersCMName:      "fleet-edge-clusters",
		},
	}
	body, _ := json.Marshal(map[string]interface{}{
		"ingress": []map[string]interface{}{
			{
				"id":             "homelab-pf",
				"method":         "port_forward",
				"wanAddressMode": "dns",
				"zone":           "homelab.example.com",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1alpha1/sites/homelab", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handlePatchSite(context.Background(), rec, req, "homelab")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var updated corev1.ConfigMap
	if err := s.Client.Get(context.Background(), client.ObjectKey{Namespace: "fleet-system", Name: "fleet-edge-clusters"}, &updated); err != nil {
		t.Fatal(err)
	}
	raw := updated.Data["clusters.yaml"]
	if !strings.Contains(raw, "wan_address_mode: dns") {
		t.Fatalf("expected synced wan mode:\n%s", raw)
	}
}
