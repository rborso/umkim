package siteprovision

import (
	"context"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
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
	hostsCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-site-hosts-homelab", Namespace: ns},
		Data: map[string]string{
			"hosts.json": `[{"name":"vm01","kind":"vm","ansible_host":"203.0.113.10","status":{"phase":"Provisioning"}}]`,
		},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-site-prov-abc123", Namespace: ns},
		Status:     batchv1.JobStatus{Succeeded: 1},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(site, hostsCM, job).Build()

	if err := RefreshRunningJob(context.Background(), c, ns, site, nil); err != nil {
		t.Fatal(err)
	}
	if site.Status.Provision.Current != nil {
		t.Fatalf("expected current cleared, got %+v", site.Status.Provision.Current)
	}
	if len(site.Status.Provision.History) != 1 || site.Status.Provision.History[0].Phase != fleetv1alpha1.ProvisionPhaseSucceeded {
		t.Fatalf("history=%+v", site.Status.Provision.History)
	}

	var updatedCM corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "fleet-site-hosts-homelab"}, &updatedCM); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(updatedCM.Data["hosts.json"], `"phase":"Ready"`) {
		t.Fatalf("hosts=%s", updatedCM.Data["hosts.json"])
	}
}
