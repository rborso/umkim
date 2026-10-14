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

// Destroying the site VMs implies the Kubernetes reset, and removing the guest
// declarations implies destroying the VMs: each level needs the one below it.
// Uninstalling the applications is independent.
func TestDeleteClusterVmCleanupImpliesLowerLevels(t *testing.T) {
	for _, tc := range []struct {
		name    string
		query   string
		k8s     bool
		vms     bool
		decls   bool
		apps    bool
		storage bool
	}{
		{name: "vms only", query: "?cleanupVms=1", k8s: true, vms: true},
		{name: "declarations only", query: "?cleanupVmDeclarations=1", k8s: true, vms: true, decls: true},
		{name: "apps only", query: "?cleanupApps=1", apps: true},
		{name: "storage only", query: "?cleanupStorage=1", storage: true},
		{name: "all five", query: "?cleanupKubernetes=1&cleanupVms=1&cleanupVmDeclarations=1&cleanupApps=1&cleanupStorage=1", k8s: true, vms: true, decls: true, apps: true, storage: true},
		{name: "none", query: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ns := "fleet-console"
			fcj := &fleetv1alpha1.FleetClusterJoin{
				ObjectMeta: metav1.ObjectMeta{Name: "lab-ha-0", Namespace: ns},
				Spec:       fleetv1alpha1.FleetClusterJoinSpec{ClusterName: "lab-ha-0", SiteId: "homelab"},
				Status:     fleetv1alpha1.FleetClusterJoinStatus{Phase: fleetv1alpha1.JoinPhaseReady},
			}
			s := &Server{
				Client: fake.NewClientBuilder().
					WithScheme(testClusterPatchScheme(t)).
					WithObjects(fcj).
					WithStatusSubresource(&fleetv1alpha1.FleetClusterJoin{}).
					Build(),
				Cfg: config.Config{Namespace: ns, RequireIdentity: false, ManagementClusterName: "management"},
			}
			req := httptest.NewRequest(http.MethodDelete, "/api/v1alpha1/clusters/lab-ha-0"+tc.query, nil)
			rec := httptest.NewRecorder()
			s.handleDeleteCluster(context.Background(), rec, req, "lab-ha-0")
			if rec.Code != http.StatusAccepted {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var got fleetv1alpha1.FleetClusterJoin
			if err := s.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "lab-ha-0"}, &got); err != nil {
				t.Fatal(err)
			}
			for _, want := range []struct {
				key string
				set bool
			}{
				{annotationCleanupKubernetes, tc.k8s},
				{annotationCleanupVms, tc.vms},
				{annotationCleanupVmDecls, tc.decls},
				{annotationCleanupApps, tc.apps},
				{annotationCleanupStorage, tc.storage},
			} {
				if v := got.Annotations[want.key] == "true"; v != want.set {
					t.Fatalf("%s=%q want set=%v", want.key, got.Annotations[want.key], want.set)
				}
			}
		})
	}
}
