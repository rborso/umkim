package reconcile

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func pipelineTestReconciler(t *testing.T) *ClusterJoinReconciler {
	t.Helper()
	scheme := deleteTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	return &ClusterJoinReconciler{Reconciler: &Reconciler{
		Client: c,
		Scheme: scheme,
		Cfg:    config.Config{Namespace: "fleet-console"},
	}}
}

func TestAutoProvisionDisabledIsIgnored(t *testing.T) {
	r := pipelineTestReconciler(t)
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "fleet-console"},
		Spec:       fleetv1alpha1.FleetClusterJoinSpec{ClusterName: "c1", SiteId: "homelab"},
	}
	_, handled, err := r.reconcileClusterAutoProvision(context.Background(), fcj, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Fatal("autoProvision=false must not be handled by the pipeline")
	}
}

func TestAutoProvisionValidatedTriggersProvision(t *testing.T) {
	scheme := deleteTestScheme(t)
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName:   "c1",
			SiteId:        "homelab",
			AutoProvision: true,
		},
		Status: fleetv1alpha1.FleetClusterJoinStatus{Phase: fleetv1alpha1.JoinPhaseValidated},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fcj).Build()
	r := &ClusterJoinReconciler{Reconciler: &Reconciler{
		Client: c,
		Scheme: scheme,
		Cfg:    config.Config{Namespace: "fleet-console"},
	}}
	_, handled, err := r.reconcileClusterAutoProvision(context.Background(), fcj, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("validated autoProvision cluster should be handled")
	}
	var got fleetv1alpha1.FleetClusterJoin
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "fleet-console", Name: "c1"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations["fleet.umkim.com/provision"] != "true" {
		t.Fatalf("expected provision annotation, got %+v", got.Annotations)
	}
}
