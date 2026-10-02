package reconcile

import (
	"context"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/wg"
)

func TestNodeWGWithReplaceControlOld_injectsRemovedCP(t *testing.T) {
	r := testClusterJoinReconciler()
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName:   "lab-ha-0",
			BootstrapHost: "172.16.10.101",
			Nodes: []fleetv1alpha1.FleetClusterJoinNodeSpec{
				{Hostname: "lab-ha-0-cp2", Address: "172.16.10.102", Role: fleetv1alpha1.NodeRoleControl},
			},
		},
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			Nodes: []fleetv1alpha1.FleetClusterJoinNodeStatus{
				{Hostname: "lab-ha-0-cp1", Address: "172.16.10.101"},
			},
			NodeOperation: &fleetv1alpha1.ClusterNodeOperationStatus{
				Type: fleetv1alpha1.NodeOpTypeReplaceControlPlane,
				Node: "lab-ha-0-cp1",
			},
		},
	}
	nodeWG := []wg.NodeWG{
		{Hostname: "lab-ha-0-cp2", Address: "172.16.10.102", WGAddress: "10.10.0.13/32", Role: fleetv1alpha1.NodeRoleControl},
	}
	out := r.nodeWGWithReplaceControlOld(fcj, fcj.Status.NodeOperation, nodeWG)
	if len(out) != 2 {
		t.Fatalf("expected old CP injected, got %d nodes", len(out))
	}
	ini := r.buildClusterInventoryINI(context.Background(), fcj, "lab-ha-0", out, true)
	if !strings.Contains(ini, "lab-ha-0-cp1 ansible_host=172.16.10.101") {
		t.Fatalf("old CP must appear in inventory for graceful leave, got:\n%s", ini)
	}
}
