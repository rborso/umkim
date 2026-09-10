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
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func seedInventoryCM(ns, site string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName(site, fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: ns,
		},
		Data: map[string]string{
			"file.guests.yml": `guests:
  hosts:
    vm01:
      vm_location: virt02
external:
  hosts:
    bm01:
      ansible_host: 203.0.113.11
`,
			"file.hypervisors.yml": `hypervisors:
  hosts:
    bm-hv01:
      ansible_host: 203.0.113.12
`,
			"_meta.status.json": `{"guests":{"vm01":{"phase":"Declared"},"bm01":{"phase":"Declared"}}}`,
		},
	}
}

func TestSiteHostCRUDDeprecated(t *testing.T) {
	ns := "fleet-console"
	site := registrySite(ns, "homelab")
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site, seedInventoryCM(ns, "homelab")).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}

	body, _ := json.Marshal(map[string]interface{}{
		"name":        "vm01",
		"kind":        "vm",
		"vm_location": "virt02",
	})
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		path := "/api/v1alpha1/sites/homelab/hosts"
		if method != http.MethodPost {
			path += "/vm01"
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		rec := httptest.NewRecorder()
		s.handleSiteByName(rec, req)
		if rec.Code != http.StatusGone {
			t.Fatalf("%s status=%d body=%s", method, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/sites/homelab/hosts", nil)
	rec := httptest.NewRecorder()
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
	if len(listed.Items) != 3 {
		t.Fatalf("items=%+v", listed.Items)
	}
}

func TestSiteHostProvisionUpdatesInventory(t *testing.T) {
	ns := "fleet-console"
	site := registrySite(ns, "homelab")
	site.Spec.Provision = &fleetv1alpha1.SiteProvisionSpec{Provider: fleetv1alpha1.ProvisionProviderNoop}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site, seedInventoryCM(ns, "homelab")).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}

	provBody, _ := json.Marshal(map[string]interface{}{
		"action":  "create_vm",
		"subject": "vm01",
		"host": map[string]string{
			"ansible_host": "203.0.113.10",
			"ansible_user": "linux",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites/homelab/provision/actions", bytes.NewReader(provBody))
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("provision status=%d body=%s", rec.Code, rec.Body.String())
	}

	var cm corev1.ConfigMap
	if err := s.Client.Get(context.Background(), client.ObjectKey{
		Namespace: ns,
		Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
	}, &cm); err != nil {
		t.Fatal(err)
	}
	var status struct {
		Guests map[string]struct {
			Phase       string `json:"phase"`
			AnsibleHost string `json:"ansible_host,omitempty"`
		} `json:"guests"`
	}
	if err := json.Unmarshal([]byte(cm.Data["_meta.status.json"]), &status); err != nil {
		t.Fatal(err)
	}
	st := status.Guests["vm01"]
	if st.Phase != fleetv1alpha1.SiteHostPhaseReady {
		t.Fatalf("phase=%q", st.Phase)
	}
	if st.AnsibleHost != "203.0.113.10" {
		t.Fatalf("ansible_host=%q", st.AnsibleHost)
	}
}

func TestSiteHostProvisionRoute(t *testing.T) {
	ns := "fleet-console"
	site := registrySite(ns, "homelab")
	site.Spec.Provision = &fleetv1alpha1.SiteProvisionSpec{Provider: fleetv1alpha1.ProvisionProviderNoop}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site, seedInventoryCM(ns, "homelab")).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites/homelab/hosts/bm01/provision", bytes.NewReader([]byte(`{"action":"install_bm"}`)))
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("provision without confirm status=%d want 400 body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites/homelab/hosts/bm01/provision", bytes.NewReader([]byte(`{"action":"install_bm","confirm":true}`)))
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
