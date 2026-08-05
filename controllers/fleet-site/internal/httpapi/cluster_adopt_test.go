package httpapi

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/overlay"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testHTTPAPIScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func TestAdoptOverlayIPCollision(t *testing.T) {
	ns := "fleet-console"
	existing := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab-ha", Namespace: ns},
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			WireguardAddress: "10.10.0.11/32",
		},
	}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testHTTPAPIScheme(t)).WithObjects(existing).Build(),
		Cfg: config.Config{
			Namespace:            ns,
			ReservedOverlayHosts: overlay.ParseReservedHosts(""),
		},
	}
	used, err := overlay.CollectUsed(context.Background(), s.Client, s.Cfg.Namespace, s.Cfg.ReservedOverlayHosts)
	if err != nil {
		t.Fatal(err)
	}
	if err := overlay.AssertAvailable(used, "10.10.0.11/32"); err == nil {
		t.Fatal("expected collision adopting already allocated overlay IP")
	}
	if err := overlay.AssertAvailable(used, "10.10.0.15/32"); err != nil {
		t.Fatalf("expected free overlay IP: %v", err)
	}
}
