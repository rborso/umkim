package routesync

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
)

func TestSortClusterTargetsStableOrder(t *testing.T) {
	targets := []fleetv1alpha1.ClusterTarget{
		{Cluster: "b", Provider: "helm-b"},
		{Cluster: "a", Provider: "helm-a"},
	}
	sortClusterTargets(targets)
	if targets[0].Cluster != "a" || targets[1].Cluster != "b" {
		t.Fatalf("unexpected order: %+v", targets)
	}
}
