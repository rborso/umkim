package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestDeleteClusterMarksDeleting(t *testing.T) {
	ns := "fleet-console"
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "lab-ha-0", Namespace: ns},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName: "lab-ha-0",
			SiteId:      "homelab",
		},
		Status: fleetv1alpha1.FleetClusterJoinStatus{Phase: fleetv1alpha1.JoinPhaseReady},
	}
	s := &Server{
		Client: fake.NewClientBuilder().
			WithScheme(testClusterPatchScheme(t)).
			WithObjects(fcj).
			WithStatusSubresource(&fleetv1alpha1.FleetClusterJoin{}).
			Build(),
		Cfg: config.Config{Namespace: ns, RequireIdentity: false, ManagementClusterName: "management"},
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1alpha1/clusters/lab-ha-0?cleanupKubernetes=1", nil)
	rec := httptest.NewRecorder()
	s.handleDeleteCluster(context.Background(), rec, req, "lab-ha-0")

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["provisioning_phase"] != fleetv1alpha1.JoinPhaseDeleting {
		t.Fatalf("phase=%q", body["provisioning_phase"])
	}
	var updated fleetv1alpha1.FleetClusterJoin
	if err := s.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "lab-ha-0"}, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != fleetv1alpha1.JoinPhaseDeleting {
		t.Fatalf("fcj phase=%q", updated.Status.Phase)
	}
	if !controllerutil.ContainsFinalizer(&updated, fleetv1alpha1.ClusterCleanupFinalizer) {
		t.Fatal("expected cluster cleanup finalizer")
	}
	if updated.Annotations[annotationCleanupKubernetes] != "true" {
		t.Fatalf("cleanup-kubernetes=%q", updated.Annotations[annotationCleanupKubernetes])
	}
}

func TestDeleteClusterAlreadyDeleting(t *testing.T) {
	ns := "fleet-console"
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "lab-ha-0",
			Namespace:  ns,
			Finalizers: []string{fleetv1alpha1.ClusterCleanupFinalizer},
		},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{ClusterName: "lab-ha-0", SiteId: "homelab"},
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			Phase:   fleetv1alpha1.JoinPhaseDeleting,
			Message: "Cluster deletion in progress",
		},
	}
	s := &Server{
		Client: fake.NewClientBuilder().
			WithScheme(testClusterPatchScheme(t)).
			WithObjects(fcj).
			WithStatusSubresource(&fleetv1alpha1.FleetClusterJoin{}).
			Build(),
		Cfg: config.Config{Namespace: ns, RequireIdentity: false, ManagementClusterName: "management"},
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1alpha1/clusters/lab-ha-0", nil)
	rec := httptest.NewRecorder()
	s.handleDeleteCluster(context.Background(), rec, req, "lab-ha-0")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
