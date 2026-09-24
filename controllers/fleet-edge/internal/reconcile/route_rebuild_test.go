package reconcile

import (
	"context"
	"testing"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFleetRoutesRebuildKey(t *testing.T) {
	route := &fleetv1alpha1.FleetAppRoute{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 3,
			Annotations: map[string]string{
				annotationFailoverGeneration: "2",
			},
		},
		Status: fleetv1alpha1.FleetAppRouteStatus{
			ActiveCluster: "lab-ext02",
		},
	}
	if got := fleetRoutesRebuildKey(route); got != "g3:f2:alab-ext02" {
		t.Fatalf("unexpected key %q", got)
	}
}

func TestShouldRebuildFleetRoutes(t *testing.T) {
	r := &RouteReconciler{}
	route := &fleetv1alpha1.FleetAppRoute{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 5,
			Annotations: map[string]string{
				annotationFleetRoutesRebuildKey: "g5:f:a",
			},
		},
	}
	if r.shouldRebuildFleetRoutes(route, false) {
		t.Fatal("expected skip when rebuild key matches generation")
	}
	if !r.shouldRebuildFleetRoutes(route, true) {
		t.Fatal("expected rebuild when spec patched")
	}
}

func TestReconcileDefersFleetRoutesRebuildWhileACMEIssuing(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	const ns = "fleet-system"
	clustersCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-edge-clusters", Namespace: ns},
		Data:       map[string]string{"clusters.yaml": "clusters: {}\n"},
	}
	route := &fleetv1alpha1.FleetAppRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "app",
			Namespace:  ns,
			Finalizers: []string{finalizerName},
		},
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			Mode:    fleetv1alpha1.RouteModeNone,
			Cluster: "lab",
			HubHost: "app.example.com",
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(route).WithObjects(clustersCM, route).Build()
	acme := &acmeCoordinator{}
	acme.issuing.Store(true)
	r := &RouteReconciler{
		Client: c,
		Config: config.Config{
			ClusterConfigMapNS:   ns,
			ClusterConfigMapName: "fleet-edge-clusters",
			FleetRoutesCMNS:      "fleet-ingress",
			FleetRoutesCMName:    "fleet-traefik-fleet-routes",
			FleetRoutesCMKey:     "fleet-routes.yml",
			RequeueSeconds:       30,
		},
		acme: acme,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(route)})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 30*time.Second {
		t.Fatalf("requeue = %s, want 30s", result.RequeueAfter)
	}
	var got fleetv1alpha1.FleetAppRoute
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(route), &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[annotationFleetRoutesRebuildKey] != "" {
		t.Fatalf("rebuild annotation = %q, want unset", got.Annotations[annotationFleetRoutesRebuildKey])
	}
	if got.Status.Phase == fleetv1alpha1.PhaseDegraded || got.Status.Phase == fleetv1alpha1.PhaseApplied {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}
