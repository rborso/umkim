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
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
`,
			"file.bare_metals.yml": `bare_metals:
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

func TestSiteHostCRUDRemoved(t *testing.T) {
	ns := "fleet-console"
	site := registrySite(ns, "homelab")
	s := &Server{
		Client: testSiteClientBuilder(t).WithObjects(site, seedInventoryCM(ns, "homelab")).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}

	body, _ := json.Marshal(map[string]interface{}{
		"name":        "vm01",
		"kind":        "vm",
		"vm_location": "virt02",
	})
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1alpha1/sites/homelab/hosts"},
		{http.MethodPatch, "/api/v1alpha1/sites/homelab/hosts/vm01"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(body))
		rec := httptest.NewRecorder()
		s.handleSiteByName(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status=%d body=%s", tc.method, rec.Code, rec.Body.String())
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
	kinds := map[string]string{}
	for _, item := range listed.Items {
		kinds[item.Name] = item.Kind
	}
	if kinds["vm01"] != fleetv1alpha1.SiteHostKindVM {
		t.Fatalf("vm01 kind=%q", kinds["vm01"])
	}
	if kinds["bm01"] != fleetv1alpha1.SiteHostKindBareMetal {
		t.Fatalf("bm01 kind=%q", kinds["bm01"])
	}
	if kinds["bm-hv01"] != fleetv1alpha1.SiteHostKindHypervisor {
		t.Fatalf("bm-hv01 kind=%q", kinds["bm-hv01"])
	}

	// DELETE removes the host's inventory records (its VM must be gone already).
	req = httptest.NewRequest(http.MethodDelete, "/api/v1alpha1/sites/homelab/hosts/vm01", nil)
	rec = httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1alpha1/sites/homelab/hosts", nil)
	rec = httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	for _, item := range listed.Items {
		if item.Name == "vm01" {
			t.Fatalf("vm01 still listed after remove: %+v", listed.Items)
		}
	}
	var got corev1.ConfigMap
	if err := s.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "fleet-site-homelab-yaidap-inventory"}, &got); err != nil {
		t.Fatal(err)
	}
	if guests := got.Data["file.guests.yml"]; strings.Contains(guests, "vm01") {
		t.Fatalf("vm01 still in guests.yml: %s", guests)
	}
}

func TestSiteHostProvisionUpdatesInventory(t *testing.T) {
	ns := "fleet-console"
	site := registrySite(ns, "homelab")
	site.Spec.Provision = &fleetv1alpha1.SiteProvisionSpec{Provider: fleetv1alpha1.ProvisionProviderNoop}
	s := &Server{
		Client: testSiteClientBuilder(t).WithObjects(site, seedInventoryCM(ns, "homelab")).Build(),
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
		Client: testSiteClientBuilder(t).WithObjects(site, seedInventoryCM(ns, "homelab")).Build(),
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
