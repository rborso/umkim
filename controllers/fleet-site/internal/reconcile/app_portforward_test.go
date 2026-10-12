package reconcile

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureTCPForwarderForwardsToClusterOverlay(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fleetv1alpha1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	ns := "fleet-console"
	app := &fleetv1alpha1.FleetApp{
		ObjectMeta: metav1.ObjectMeta{Name: "gitea", Namespace: ns, UID: "uid-1"},
		Spec: fleetv1alpha1.FleetAppSpec{
			ClusterName: "rp-test",
			Chart:       "gitea",
			Route: fleetv1alpha1.FleetAppRouteIntent{
				Protocol:    fleetv1alpha1.AppRouteProtocolTCP,
				ForwardPort: 2222,
				ServicePort: 32222,
			},
		},
	}
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "rp-test", Namespace: ns},
		Status:     fleetv1alpha1.FleetClusterJoinStatus{WireguardAddress: "10.10.0.14/32"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(app, fcj).Build()
	r := &AppReconciler{Reconciler: &Reconciler{Client: c, Scheme: scheme, Cfg: config.Config{Namespace: ns}}}

	if err := r.ensureTCPForwarders(context.Background(), app, "gitea"); err != nil {
		t.Fatal(err)
	}
	var dep appsv1.Deployment
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: tcpForwardName("gitea")}, &dep); err != nil {
		t.Fatal(err)
	}
	if !dep.Spec.Template.Spec.HostNetwork {
		t.Fatal("tcp forwarder must listen on the host network")
	}
	args := dep.Spec.Template.Spec.Containers[0].Args
	want := []string{"TCP-LISTEN:2222,fork,reuseaddr", "TCP:10.10.0.14:32222"}
	if len(args) != 2 || args[0] != want[0] || args[1] != want[1] {
		t.Fatalf("socat args = %v, want %v", args, want)
	}
	if dep.Labels[tcpForwardLabel] != "true" || dep.Labels[appLabel] != "gitea" {
		t.Fatalf("forwarder labels = %v", dep.Labels)
	}
}

func TestEnsureTCPForwardersRequiresForwardPort(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fleetv1alpha1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	ns := "fleet-console"
	app := &fleetv1alpha1.FleetApp{
		ObjectMeta: metav1.ObjectMeta{Name: "gitea", Namespace: ns, UID: "uid-1"},
		Spec: fleetv1alpha1.FleetAppSpec{
			ClusterName: "rp-test",
			Chart:       "gitea",
			Route: fleetv1alpha1.FleetAppRouteIntent{
				Protocol:    fleetv1alpha1.AppRouteProtocolTCP,
				ServicePort: 32222,
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(app).Build()
	r := &AppReconciler{Reconciler: &Reconciler{Client: c, Scheme: scheme, Cfg: config.Config{Namespace: ns}}}
	if err := r.ensureTCPForwarders(context.Background(), app, "gitea"); err == nil {
		t.Fatal("expected an error when forwardPort is missing")
	}
}
