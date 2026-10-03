package v1alpha1

import "testing"

func TestPlacementClustersFallback(t *testing.T) {
	r := FleetAppRoute{
		Spec: FleetAppRouteSpec{
			Cluster: "virt01",
			Service: ServiceRef{Name: "app-svc", Namespace: "default", Port: 80},
		},
	}
	got := r.PlacementClusters()
	if len(got) != 1 || got[0].Cluster != "virt01" || got[0].Service.Name != "app-svc" {
		t.Fatalf("unexpected fallback targets: %+v", got)
	}
}

func TestClusterNamesForCleanup(t *testing.T) {
	r := FleetAppRoute{
		Spec: FleetAppRouteSpec{
			Cluster:        "virt01",
			StandbyCluster: "virt02",
			ClusterTargets: []ClusterTarget{
				{Cluster: "virt01", Service: ServiceRef{Name: "a", Port: 80}},
				{Cluster: "virt02", Service: ServiceRef{Name: "b", Port: 80}},
			},
		},
	}
	names := r.ClusterNamesForCleanup()
	if len(names) != 2 {
		t.Fatalf("expected 2 clusters, got %v", names)
	}
}
