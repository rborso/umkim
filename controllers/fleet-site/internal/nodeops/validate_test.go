package nodeops

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestValidatePreAddWorker(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Status: fleetv1alpha1.FleetClusterJoinStatus{Phase: fleetv1alpha1.JoinPhaseReady},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			Nodes: []fleetv1alpha1.FleetClusterJoinNodeSpec{{
				Hostname: "cp1",
				Address:  "10.0.0.1",
				Role:     fleetv1alpha1.NodeRoleControl,
			}},
		},
	}
	res := Validate(PhasePre, fcj, &Request{
		Type:    fleetv1alpha1.NodeOpTypeAddWorker,
		Address: "10.0.0.2",
	}, nil, false)
	if !res.OK {
		t.Fatalf("expected ok pre-validate, got %#v", res)
	}
	res = Validate(PhasePre, fcj, &Request{
		Type:    fleetv1alpha1.NodeOpTypeAddWorker,
		Address: "10.0.0.1",
	}, nil, false)
	if res.OK {
		t.Fatalf("expected duplicate address to fail")
	}
}

func TestValidatePreReplaceControlSingleNodeRejected(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Status: fleetv1alpha1.FleetClusterJoinStatus{Phase: fleetv1alpha1.JoinPhaseReady},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			Topology: fleetv1alpha1.ClusterTopologySingleNode,
			Nodes: []fleetv1alpha1.FleetClusterJoinNodeSpec{{
				Hostname: "cp1",
				Address:  "10.0.0.1",
				Role:     fleetv1alpha1.NodeRoleControl,
			}},
		},
	}
	res := Validate(PhasePre, fcj, &Request{
		Type:    fleetv1alpha1.NodeOpTypeReplaceControlPlane,
		OldNode: "cp1",
		NewNode: &NewNode{Address: "10.0.0.9"},
	}, nil, false)
	if res.OK {
		t.Fatalf("expected replaceControlPlane rejected for single_node, got %#v", res)
	}
}

func TestValidatePreReplaceControlHAPasses(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Status: fleetv1alpha1.FleetClusterJoinStatus{Phase: fleetv1alpha1.JoinPhaseReady},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			Topology: fleetv1alpha1.ClusterTopologyHA3Node,
			Nodes: []fleetv1alpha1.FleetClusterJoinNodeSpec{
				{Hostname: "cp1", Address: "10.0.0.1", Role: fleetv1alpha1.NodeRoleControl},
				{Hostname: "cp2", Address: "10.0.0.2", Role: fleetv1alpha1.NodeRoleControl},
				{Hostname: "cp3", Address: "10.0.0.3", Role: fleetv1alpha1.NodeRoleControl},
			},
		},
	}
	nodes := []NodeInfo{
		{Name: "cp1", Roles: []string{"control-plane"}, Ready: true},
		{Name: "cp2", Roles: []string{"control-plane"}, Ready: true},
		{Name: "cp3", Roles: []string{"control-plane"}, Ready: true},
	}
	res := Validate(PhasePre, fcj, &Request{
		Type:    fleetv1alpha1.NodeOpTypeReplaceControlPlane,
		OldNode: "cp1",
		NewNode: &NewNode{Address: "10.0.0.9"},
	}, nodes, false)
	if !res.OK {
		t.Fatalf("expected HA replaceControlPlane to pass, got %#v", res)
	}
}

func TestValidatePostRemoveWorker(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			Nodes: []fleetv1alpha1.FleetClusterJoinNodeSpec{{
				Hostname: "cp1",
				Address:  "10.0.0.1",
				Role:     fleetv1alpha1.NodeRoleControl,
			}},
		},
	}
	res := Validate(PhasePost, fcj, &Request{
		Type: fleetv1alpha1.NodeOpTypeRemoveWorker,
		Node: "w1",
	}, []NodeInfo{{Name: "cp1", Roles: []string{"control-plane"}, Ready: true}}, false)
	if !res.OK {
		t.Fatalf("expected ok post-validate, got %#v", res)
	}
}
