package siteinventory

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestInventorySyncInProgressFromMeta(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	ns := "fleet-console"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: ns,
		},
		Data: map[string]string{
			"_meta.meta.json": `{"syncPhase":"syncing"}`,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	busy, err := InventorySyncInProgress(context.Background(), c, ns, "homelab")
	if err != nil {
		t.Fatal(err)
	}
	if !busy {
		t.Fatal("expected syncing")
	}
}

func TestInventorySyncInProgressFromRunningJob(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)
	ns := "fleet-console"
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      JobPrefix + "abc123",
			Namespace: ns,
			Labels:    map[string]string{"fleet-site": "homelab"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
	busy, err := InventorySyncInProgress(context.Background(), c, ns, "homelab")
	if err != nil {
		t.Fatal(err)
	}
	if !busy {
		t.Fatal("expected active inventory job")
	}
}

func TestEnqueueInventorySyncSkipsWhenBusy(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = fleetv1alpha1.AddToScheme(scheme)
	ns := "fleet-console"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: ns,
		},
		Data: map[string]string{
			"_meta.meta.json": `{"syncPhase":"syncing"}`,
		},
	}
	fs := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			Provision: &fleetv1alpha1.SiteProvisionSpec{
				InventoryPath: "/opt/yaidap/inventory",
				Jumphost:      "jumphost.example",
				SSHSecretRef:  "ssh",
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm, fs).Build()
	jobName, err := EnqueueInventorySync(context.Background(), c, config.Config{Namespace: ns}, fs)
	if err != nil {
		t.Fatal(err)
	}
	if jobName != "" {
		t.Fatalf("expected skip, got job %q", jobName)
	}
}
