package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestInventoryImportFailureIsVisible(t *testing.T) {
	ns := "fleet-console"
	const jobName = "fleet-site-inv-import-homelab"
	logs := `FLEET_INVENTORY_IMPORT={"catalog":{"networks":[{"name":""}]}}`
	kube := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(logs))
			return
		}
		if strings.Contains(r.URL.Path, "/pods") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"kind":"PodList","apiVersion":"v1","items":[{"metadata":{"name":"import-pod","namespace":"` + ns + `"}}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer kube.Close()

	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName: "homelab",
			SiteId:   "homelab",
			Provision: &fleetv1alpha1.SiteProvisionSpec{
				Provider: fleetv1alpha1.ProvisionProviderYaidap,
			},
		},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
			Labels:    map[string]string{"fleet-site": "homelab"},
		},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	s := &Server{
		Client: fake.NewClientBuilder().
			WithScheme(testSiteHTTPScheme(t)).
			WithStatusSubresource(job).
			WithObjects(site, job).
			Build(),
		Cfg:        config.Config{Namespace: ns, RequireIdentity: false},
		RESTConfig: &rest.Config{Host: kube.URL},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/sites/homelab/provision/inventory", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Meta struct {
			SyncPhase      string `json:"syncPhase"`
			LastImportNote string `json:"lastImportNote"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Meta.SyncPhase != "failed" {
		t.Fatalf("syncPhase=%q body=%s", body.Meta.SyncPhase, rec.Body.String())
	}
	if !strings.Contains(body.Meta.LastImportNote, "catalog.networks") {
		t.Fatalf("note=%q", body.Meta.LastImportNote)
	}
	var jobs batchv1.JobList
	if err := s.Client.List(context.Background(), &jobs, client.InNamespace(ns)); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 0 {
		t.Fatalf("job still present: %d", len(jobs.Items))
	}
}
