package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSiteInventoryFilePutGet(t *testing.T) {
	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "homelab",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
			Provision: &fleetv1alpha1.SiteProvisionSpec{
				Provider:      fleetv1alpha1.ProvisionProviderYaidap,
				Jumphost:      "linux@jumphost.example:22",
				SSHSecretRef:  "fleet-site-ssh",
				InventoryPath: "/home/linux/yaidap/inventory/lab",
			},
		},
	}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}

	guestsYAML := `guests:
  hosts:
    lab-single-1-0:
      ansible_host: 172.16.10.161
      vm_location: least-utilized
external:
  hosts: {}
`
	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/sites/homelab/provision/inventory/files/guests.yml", strings.NewReader(guestsYAML))
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1alpha1/sites/homelab/provision/inventory/files/guests.yml", nil)
	rec = httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "lab-single-1-0") {
		t.Fatalf("body=%s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1alpha1/sites/homelab/provision/inventory/revisions?limit=5", nil)
	rec = httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("revisions status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSiteInventoryGuestMutationsRemoved(t *testing.T) {
	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "homelab",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
			Provision: &fleetv1alpha1.SiteProvisionSpec{
				Provider: fleetv1alpha1.ProvisionProviderYaidap,
			},
		},
	}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites/homelab/provision/inventory/guests", bytes.NewReader([]byte(`{"name":"x","spec":{}}`)))
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSiteInventoryRejectNoopProvider(t *testing.T) {
	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "homelab",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
			Provision:        &fleetv1alpha1.SiteProvisionSpec{Provider: fleetv1alpha1.ProvisionProviderNoop},
		},
	}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(site).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/sites/homelab/provision/inventory", nil)
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
