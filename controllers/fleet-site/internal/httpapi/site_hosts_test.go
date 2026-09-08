package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSiteHostCRUD(t *testing.T) {
	ns := "fleet-console"
	site := registrySite(ns, "homelab")
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}

	body, _ := json.Marshal(map[string]interface{}{
		"name":         "vm01",
		"kind":         "vm",
		"vm_location":  "virt02",
		"ansible_user": "linux",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites/homelab/hosts", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1alpha1/sites/homelab/hosts", nil)
	rec = httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Items []fleetv1alpha1.SiteHostRecord `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 || listed.Items[0].Name != "vm01" {
		t.Fatalf("items=%+v", listed.Items)
	}

	patch, _ := json.Marshal(map[string]string{"ansible_host": "203.0.113.10"})
	req = httptest.NewRequest(http.MethodPatch, "/api/v1alpha1/sites/homelab/hosts/vm01", bytes.NewReader(patch))
	rec = httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1alpha1/sites/homelab/hosts/vm01", nil)
	rec = httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSiteHostProvisionUpdatesInventory(t *testing.T) {
	ns := "fleet-console"
	site := registrySite(ns, "homelab")
	site.Spec.Provision = &fleetv1alpha1.SiteProvisionSpec{Provider: fleetv1alpha1.ProvisionProviderNoop}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}

	createBody, _ := json.Marshal(map[string]interface{}{
		"name": "vm01",
		"kind": "vm",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites/homelab/hosts", bytes.NewReader(createBody))
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}

	provBody, _ := json.Marshal(map[string]interface{}{
		"action":  "create_vm",
		"subject": "vm01",
		"host": map[string]string{
			"ansible_host": "203.0.113.10",
			"ansible_user": "linux",
		},
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites/homelab/provision/actions", bytes.NewReader(provBody))
	rec = httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("provision status=%d body=%s", rec.Code, rec.Body.String())
	}

	var cm corev1.ConfigMap
	if err := s.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "fleet-site-hosts-homelab"}, &cm); err != nil {
		t.Fatal(err)
	}
	var hosts []fleetv1alpha1.SiteHostRecord
	if err := json.Unmarshal([]byte(cm.Data["hosts.json"]), &hosts); err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Fatalf("hosts=%+v", hosts)
	}
	if hosts[0].Status.Phase != fleetv1alpha1.SiteHostPhaseReady {
		t.Fatalf("phase=%q", hosts[0].Status.Phase)
	}
	if hosts[0].AnsibleHost != "203.0.113.10" {
		t.Fatalf("ansible_host=%q", hosts[0].AnsibleHost)
	}
}

func TestSiteHostProvisionRoute(t *testing.T) {
	ns := "fleet-console"
	site := registrySite(ns, "homelab")
	site.Spec.Provision = &fleetv1alpha1.SiteProvisionSpec{Provider: fleetv1alpha1.ProvisionProviderNoop}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}

	createBody, _ := json.Marshal(map[string]interface{}{
		"name":         "bm01",
		"kind":         "bare_metal",
		"ansible_host": "203.0.113.11",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites/homelab/hosts", bytes.NewReader(createBody))
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites/homelab/hosts/bm01/provision", bytes.NewReader([]byte(`{}`)))
	rec = httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("provision without confirm status=%d want 400 body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites/homelab/hosts/bm01/provision", bytes.NewReader([]byte(`{"confirm":true}`)))
	rec = httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("provision status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func registrySite(ns, name string) *fleetv1alpha1.FleetSite {
	return &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         name,
			SiteId:           name,
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
		},
	}
}
