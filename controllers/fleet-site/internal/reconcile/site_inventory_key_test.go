package reconcile

import (
	"context"
	"testing"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileRegistrySiteLoadsInventoryByCRName(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "ho",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
			Provision: &fleetv1alpha1.SiteProvisionSpec{
				Provider: fleetv1alpha1.ProvisionProviderYaidap,
			},
		},
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: ns,
		},
		Data: map[string]string{
			"file.guests.yml": `guests:
  hosts:
    vm01:
      ansible_host: 198.51.100.20
`,
			"file.lab.yml": `lab:
  hosts:
    vm01:
`,
			"file.group_vars/lab.yml": "topology: single_node\nmode: enroll\n",
			"_meta.status.json":       `{"guests":{"vm01":{"phase":"Ready","ansible_host":"198.51.100.20"}}}`,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(site, cm).Build()
	r := &Reconciler{Client: c, Scheme: scheme, Cfg: config.Config{Namespace: ns}}

	res, err := r.reconcileRegistrySiteHostClusters(context.Background(), site)
	if err != nil {
		t.Fatal(err)
	}
	if res.RequeueAfter != 15*time.Second {
		t.Fatalf("requeue=%s", res.RequeueAfter)
	}
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := c.Get(context.Background(), types.NamespacedName{Name: "lab", Namespace: ns}, &fcj); err != nil {
		t.Fatalf("join not created from homelab inventory: %v", err)
	}
	if fcj.Spec.SiteId != "ho" {
		t.Fatalf("siteId=%q", fcj.Spec.SiteId)
	}
}
