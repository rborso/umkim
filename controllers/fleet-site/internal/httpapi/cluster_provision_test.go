package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestProvisionReadyClusterConflict(t *testing.T) {
	ns := "fleet-console"
	fcj := readyJoin("lab")
	mc := availableManagedCluster("lab")
	s := provisionServer(t, ns, fcj, mc)

	rec := postProvision(s, "lab")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "already provisioned") {
		t.Fatalf("body=%s", rec.Body.String())
	}
	var updated fleetv1alpha1.FleetClusterJoin
	if err := s.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "lab"}, &updated); err != nil {
		t.Fatal(err)
	}
	if _, ok := updated.Annotations["fleet.umkim.com/provision"]; ok {
		t.Fatal("provision annotation was set")
	}
}

func TestProvisionReadyClusterWithoutManagedCluster(t *testing.T) {
	ns := "fleet-console"
	fcj := readyJoin("lab")
	s := provisionServer(t, ns, fcj)

	rec := postProvision(s, "lab")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var updated fleetv1alpha1.FleetClusterJoin
	if err := s.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "lab"}, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Annotations["fleet.umkim.com/provision"] != "true" {
		t.Fatalf("annotation=%q", updated.Annotations["fleet.umkim.com/provision"])
	}
}

func availableManagedCluster(name string) *unstructured.Unstructured {
	mc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cluster.open-cluster-management.io/v1",
		"kind":       "ManagedCluster",
		"metadata":   map[string]interface{}{"name": name},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "ManagedClusterConditionAvailable", "status": "True"},
			},
		},
	}}
	mc.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	mc.SetName(name)
	return mc
}

func provisionServer(t *testing.T, ns string, objs ...client.Object) *Server {
	t.Helper()
	scheme := testClusterPatchScheme(t)
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	}, &unstructured.Unstructured{})
	return &Server{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&fleetv1alpha1.FleetClusterJoin{}).
			WithObjects(objs...).
			Build(),
		Cfg: config.Config{Namespace: ns, RequireIdentity: false},
	}
}

func postProvision(s *Server, name string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/clusters/"+name+"/provision", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}
