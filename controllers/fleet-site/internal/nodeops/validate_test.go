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
