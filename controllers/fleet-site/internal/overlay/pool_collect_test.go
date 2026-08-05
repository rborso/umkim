package overlay

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testOverlayScheme(t *testing.T) *runtime.Scheme {
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

func TestCollectUsedFromClusterJoinAndBootstrap(t *testing.T) {
	ns := "fleet-console"
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab-ha", Namespace: ns},
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			WireguardAddress: "10.10.0.10/32",
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-cluster-homelab-ha-bootstrap", Namespace: ns},
		Data: map[string][]byte{
			"nodes-wg.json": []byte(`[
				{"hostname":"cp1","address":"198.51.100.81","wgAddress":"10.10.0.10/32"},
				{"hostname":"cp2","address":"198.51.100.82","wgAddress":"10.10.0.12/32"}
			]`),
		},
	}
	c := fake.NewClientBuilder().WithScheme(testOverlayScheme(t)).WithObjects(fcj, secret).Build()
	used, err := CollectUsed(context.Background(), c, ns, ParseReservedHosts(""))
	if err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"10.10.0.1", "10.10.0.11", "10.10.0.10", "10.10.0.12"} {
		if !used[ip] {
			t.Fatalf("expected %s used", ip)
		}
	}
	if used["10.10.0.20"] {
		t.Fatal("10.10.0.20 should be free")
	}
}
