package reconcile

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/wg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
				Type:  fleetv1alpha1.NodeOpTypeReplaceControlPlane,
				Node:  "lab-ha-0-cp1",
				Phase: fleetv1alpha1.NodeOpPhaseRunning,
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

func TestNodeWGWithReplaceControlOld_usesPersistedAddressWhenSpecStatusDropped(t *testing.T) {
	r := testClusterJoinReconciler()
	req := nodeOperationRequest{
		Type:           fleetv1alpha1.NodeOpTypeReplaceControlPlane,
		OldNode:        "lab-ha-0-cp3",
		OldNodeAddress: "172.16.10.142",
		NewNode:        &nodeOpNewNode{Address: "172.16.10.143", Hostname: "lab-ha-0-cp4"},
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotationNodeOperationActive: string(raw),
			},
		},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName:   "lab-ha-0",
			BootstrapHost: "172.16.10.101",
			Nodes: []fleetv1alpha1.FleetClusterJoinNodeSpec{
				{Hostname: "lab-ha-0-cp1", Address: "172.16.10.101", Role: fleetv1alpha1.NodeRoleControl},
				{Hostname: "lab-ha-0-cp2", Address: "172.16.10.102", Role: fleetv1alpha1.NodeRoleControl},
				{Hostname: "lab-ha-0-cp4", Address: "172.16.10.143", Role: fleetv1alpha1.NodeRoleControl},
			},
		},
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			// Spec rewrite + syncNodeWGForSpec already dropped old CP from status.
			Nodes: []fleetv1alpha1.FleetClusterJoinNodeStatus{
				{Hostname: "lab-ha-0-cp1", Address: "172.16.10.101"},
				{Hostname: "lab-ha-0-cp2", Address: "172.16.10.102"},
				{Hostname: "lab-ha-0-cp4", Address: "172.16.10.143"},
			},
			NodeOperation: &fleetv1alpha1.ClusterNodeOperationStatus{
				Type:  fleetv1alpha1.NodeOpTypeReplaceControlPlane,
				Node:  "lab-ha-0-cp3",
				Phase: fleetv1alpha1.NodeOpPhaseRunning,
			},
		},
	}
	nodeWG := []wg.NodeWG{
		{Hostname: "lab-ha-0-cp1", Address: "172.16.10.101", Role: fleetv1alpha1.NodeRoleControl},
		{Hostname: "lab-ha-0-cp2", Address: "172.16.10.102", Role: fleetv1alpha1.NodeRoleControl},
		{Hostname: "lab-ha-0-cp4", Address: "172.16.10.143", Role: fleetv1alpha1.NodeRoleControl},
	}
	out := r.nodeWGWithReplaceControlOld(fcj, nil, nodeWG)
	if len(out) != 4 {
		t.Fatalf("expected old CP injected from persisted OldNodeAddress, got %d nodes", len(out))
	}
	ini := r.buildClusterInventoryINI(context.Background(), fcj, "lab-ha-0", out, true)
	if !strings.Contains(ini, "lab-ha-0-cp3 ") || !strings.Contains(ini, "172.16.10.142") {
		t.Fatalf("persisted old CP address must appear in inventory, got:\n%s", ini)
	}
	if !strings.Contains(ini, "lab-ha-0-cp3 ansible_host=172.16.10.142") && !strings.Contains(ini, "fleet_node_mgmt_address=172.16.10.142") {
		t.Fatalf("old CP must be reachable via ansible_host or fleet_node_mgmt_address, got:\n%s", ini)
	}
}

func TestNodeWGWithReplaceControlOld_skipsTerminalPhases(t *testing.T) {
	r := testClusterJoinReconciler()
	req := nodeOperationRequest{
		Type:           fleetv1alpha1.NodeOpTypeReplaceControlPlane,
		OldNode:        "lab-ha-0-cp3",
		OldNodeAddress: "172.16.10.142",
	}
	raw, _ := json.Marshal(req)
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{annotationNodeOperationActive: string(raw)}},
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			NodeOperation: &fleetv1alpha1.ClusterNodeOperationStatus{
				Type:  fleetv1alpha1.NodeOpTypeReplaceControlPlane,
				Node:  "lab-ha-0-cp3",
				Phase: fleetv1alpha1.NodeOpPhaseSucceeded,
			},
		},
	}
	nodeWG := []wg.NodeWG{{Hostname: "lab-ha-0-cp1", Address: "172.16.10.101", Role: fleetv1alpha1.NodeRoleControl}}
	out := r.nodeWGWithReplaceControlOld(fcj, nil, nodeWG)
	if len(out) != 1 {
		t.Fatalf("terminal phase must not keep injecting old CP, got %d", len(out))
	}
}
