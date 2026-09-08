package siteprovision

import (
	"context"
	"encoding/json"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRefreshRunningJobSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fleetv1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "homelab",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
		},
		Status: fleetv1alpha1.FleetSiteStatus{
			Provision: &fleetv1alpha1.SiteProvisionStatus{
				Current: &fleetv1alpha1.SiteProvisionActionRecord{
					ID:       "abc123",
					Action:   "create_vm",
					Subject:  "vm01",
					Phase:    fleetv1alpha1.ProvisionPhaseRunning,
					JobName:  "fleet-site-prov-abc123",
					Provider: fleetv1alpha1.ProvisionProviderYaidap,
				},
			},
		},
	}
	invCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: ns,
		},
		Data: map[string]string{
			"file.guests.yml": `guests:
  hosts:
    vm01:
      ansible_host: 203.0.113.10
`,
			"_meta.status.json": `{"guests":{"vm01":{"phase":"Provisioning"}}}`,
		},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-site-prov-abc123", Namespace: ns},
		Status:     batchv1.JobStatus{Succeeded: 1},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(site, invCM, job).Build()

	if err := RefreshRunningJob(context.Background(), c, ns, site, nil, nil); err != nil {
		t.Fatal(err)
	}
	if site.Status.Provision.Current != nil {
		t.Fatalf("expected current cleared, got %+v", site.Status.Provision.Current)
	}
	if len(site.Status.Provision.History) != 1 || site.Status.Provision.History[0].Phase != fleetv1alpha1.ProvisionPhaseSucceeded {
		t.Fatalf("history=%+v", site.Status.Provision.History)
	}

	var updatedCM corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Namespace: ns,
		Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
	}, &updatedCM); err != nil {
		t.Fatal(err)
	}
	var status struct {
		Guests map[string]struct {
			Phase string `json:"phase"`
		} `json:"guests"`
	}
	if err := json.Unmarshal([]byte(updatedCM.Data["_meta.status.json"]), &status); err != nil {
		t.Fatal(err)
	}
	if status.Guests["vm01"].Phase != fleetv1alpha1.SiteHostPhaseReady {
		t.Fatalf("status=%+v", status.Guests["vm01"])
	}
}
