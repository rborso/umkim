package reconcile

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
