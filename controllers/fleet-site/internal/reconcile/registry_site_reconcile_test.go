package reconcile

import (
	"context"
	"testing"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileRegistrySiteSetsReadyDespiteClusterPollRequeue(t *testing.T) {
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
			Provision: &fleetv1alpha1.SiteProvisionSpec{
				Provider:      fleetv1alpha1.ProvisionProviderYaidap,
				Jumphost:      "linux@jump.example:22",
				InventoryPath: "/inventory/homelab",
				SSHSecretRef:  "fleet-site-homelab-provision-ssh",
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(site).WithStatusSubresource(site).Build()
	r := &Reconciler{
		Client: c,
		Scheme: scheme,
		Cfg: config.Config{
			Namespace:          ns,
			RequeueIntervalSec: 30,
		},
	}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "homelab", Namespace: ns}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter <= 0 {
		t.Fatalf("expected periodic cluster-apply requeue, got %v", res.RequeueAfter)
	}
	if res.RequeueAfter != 15*time.Second {
		t.Fatalf("expected 15s cluster poll requeue, got %v", res.RequeueAfter)
	}

	var got fleetv1alpha1.FleetSite
	if err := c.Get(context.Background(), types.NamespacedName{Name: "homelab", Namespace: ns}, &got); err != nil {
		t.Fatalf("Get FleetSite: %v", err)
	}
	if got.Status.Phase != phaseReady {
		t.Fatalf("status.phase=%q, want %q", got.Status.Phase, phaseReady)
	}
	if got.Status.Message != "Registry site (datacenter metadata)" {
		t.Fatalf("status.message=%q", got.Status.Message)
	}
}
