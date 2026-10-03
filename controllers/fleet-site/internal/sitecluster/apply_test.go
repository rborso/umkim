package sitecluster

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitehosts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestApplyEnrollSingleNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fleetv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "homelab",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(site).Build()
	cfg := config.Config{Namespace: ns}

	rec := fleetv1alpha1.SiteHostRecord{
		Name:        "vm01",
		Kind:        fleetv1alpha1.SiteHostKindVM,
		AnsibleHost: "203.0.113.10",
		ClusterIntent: &fleetv1alpha1.SiteHostClusterIntent{
			Mode:        fleetv1alpha1.SiteHostClusterModeEnroll,
			ClusterName: "vm01-0",
		},
		Status: fleetv1alpha1.SiteHostStatus{
			Phase: fleetv1alpha1.SiteHostPhaseReady,
		},
	}
	if err := sitehosts.Put(context.Background(), c, ns, "homelab", rec); err != nil {
		t.Fatal(err)
	}
	if err := TryApplyHost(context.Background(), c, cfg, "homelab", "vm01"); err != nil {
		t.Fatal(err)
	}
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "vm01-0"}, &fcj); err != nil {
		t.Fatalf("fcj missing: %v", err)
	}
	if fcj.Spec.BootstrapHost != "203.0.113.10" {
		t.Fatalf("bootstrap=%q", fcj.Spec.BootstrapHost)
	}
	if fcj.Spec.MinRootDiskGi != ProvisionedVMMinRootDiskGi {
		t.Fatalf("minRootDiskGi=%d want %d", fcj.Spec.MinRootDiskGi, ProvisionedVMMinRootDiskGi)
	}
	updated, ok, err := sitehosts.Get(context.Background(), c, ns, "homelab", "vm01")
	if err != nil || !ok {
		t.Fatal(err)
	}
	if updated.Status.Cluster == nil || updated.Status.Cluster.Phase != fleetv1alpha1.SiteHostClusterPhaseApplying {
		t.Fatalf("cluster status=%+v", updated.Status.Cluster)
	}
}
