package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testClusterPatchScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func TestPatchClusterJumphost(t *testing.T) {
	ns := "fleet-console"
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "lab-0", Namespace: ns},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName: "lab-0",
			SiteId:      "homelab",
		},
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			Phase: fleetv1alpha1.JoinPhasePending,
		},
	}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testClusterPatchScheme(t)).WithObjects(fcj).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}
	body, _ := json.Marshal(map[string]string{"jumphost": "jump@203.0.113.1:22"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1alpha1/clusters/lab-0", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handlePatchCluster(context.Background(), rec, req, "lab-0")

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var summary clusterSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Jumphost != "jump@203.0.113.1:22" {
		t.Fatalf("summary jumphost=%q", summary.Jumphost)
	}
	var updated fleetv1alpha1.FleetClusterJoin
	if err := s.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "lab-0"}, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Spec.Jumphost != "jump@203.0.113.1:22" {
		t.Fatalf("fcj jumphost=%q", updated.Spec.Jumphost)
	}
}

func TestPatchClusterInvalidJumphost(t *testing.T) {
	ns := "fleet-console"
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "lab-0", Namespace: ns},
		Spec:       fleetv1alpha1.FleetClusterJoinSpec{ClusterName: "lab-0", SiteId: "homelab"},
		Status:     fleetv1alpha1.FleetClusterJoinStatus{Phase: fleetv1alpha1.JoinPhasePending},
	}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testClusterPatchScheme(t)).WithObjects(fcj).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: false},
	}
	body, _ := json.Marshal(map[string]string{"jumphost": "not-valid"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1alpha1/clusters/lab-0", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handlePatchCluster(context.Background(), rec, req, "lab-0")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rec.Code, rec.Body.String())
	}
}

func TestClusterSummaryFromJoinIncludesJumphost(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName: "lab-0",
			SiteId:      "homelab",
			Jumphost:    "jump@203.0.113.1:22",
		},
		Status: fleetv1alpha1.FleetClusterJoinStatus{Phase: fleetv1alpha1.JoinPhaseReady},
	}
	cs := clusterSummaryFromJoin(fcj)
	if cs.Jumphost != "jump@203.0.113.1:22" {
		t.Fatalf("jumphost=%q", cs.Jumphost)
	}
}
