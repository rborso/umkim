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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestSiteProvisionNoopAction(t *testing.T) {
	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "homelab",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
			Provision: &fleetv1alpha1.SiteProvisionSpec{
				Provider: fleetv1alpha1.ProvisionProviderNoop,
			},
		},
	}
	s := &Server{
		Client: testSiteClientBuilder(t).WithObjects(site, seedInventoryCM(ns, "homelab")).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}

	body, _ := json.Marshal(map[string]interface{}{
		"action":  "create_vm",
		"subject": "vm01",
		"host": map[string]string{
			"ansible_host": "203.0.113.10",
			"ansible_user": "linux",
			"osFamily":     "rocky",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites/homelab/provision/actions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out fleetv1alpha1.SiteProvisionActionRecord
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Phase != fleetv1alpha1.ProvisionPhaseSucceeded {
		t.Fatalf("phase=%q", out.Phase)
	}
	if out.Host == nil || out.Host.AnsibleHost != "203.0.113.10" {
		t.Fatalf("host=%+v", out.Host)
	}

	var updated fleetv1alpha1.FleetSite
	if err := s.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "homelab"}, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Provision == nil || updated.Status.Provision.LastActionID != out.ID {
		t.Fatalf("status not updated: %+v", updated.Status.Provision)
	}
}

func TestSiteProvisionRejectUnknownProvider(t *testing.T) {
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
		Client: testSiteClientBuilder(t).WithObjects(site).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}
	body, _ := json.Marshal(map[string]string{"provider": "bogus"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/sites/homelab/provision", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rec.Code, rec.Body.String())
	}
}

func TestSiteProvisionConfirmRequired(t *testing.T) {
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
		Client: testSiteClientBuilder(t).WithObjects(site, seedInventoryCM(ns, "homelab")).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}
	body, _ := json.Marshal(map[string]interface{}{
		"action":  "install_bm",
		"subject": "bm01",
		"host":    map[string]string{"ansible_host": "203.0.113.11"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/sites/homelab/provision/actions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetSiteProvisionBindingDefaultNoop(t *testing.T) {
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
		Client: testSiteClientBuilder(t).WithObjects(site).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/sites/homelab/provision", nil)
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out siteProvisionBindingView
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Provider != fleetv1alpha1.ProvisionProviderNoop {
		t.Fatalf("provider=%q", out.Provider)
	}
}
