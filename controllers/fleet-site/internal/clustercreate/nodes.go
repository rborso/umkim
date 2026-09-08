package clustercreate

import (
	"fmt"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

// NormalizeNodes validates and fills node specs for cluster create.
func NormalizeNodes(clusterName, topology, bootstrapHost string, nodes []fleetv1alpha1.FleetClusterJoinNodeSpec) ([]fleetv1alpha1.FleetClusterJoinNodeSpec, error) {
	if topology == fleetv1alpha1.ClusterTopologySingleNode {
		if len(nodes) == 0 {
			hn := clusterName
			if hn == "" {
				hn = "node"
			}
			return []fleetv1alpha1.FleetClusterJoinNodeSpec{{
				Hostname: hn,
				Address:  bootstrapHost,
				Role:     fleetv1alpha1.NodeRoleControl,
			}}, nil
		}
		if len(nodes) != 1 {
			return nil, fmt.Errorf("single_node requires exactly 1 node")
		}
		if nodes[0].Address == "" {
			return nil, fmt.Errorf("node requires an address")
		}
		if nodes[0].EffectiveRole() == fleetv1alpha1.NodeRoleWorker {
			return nil, fmt.Errorf("single_node does not support worker nodes")
		}
		if nodes[0].Hostname == "" {
			nodes[0].Hostname = clusterName
		}
		nodes[0].Role = fleetv1alpha1.NodeRoleControl
		return nodes, nil
	}
	if topology != fleetv1alpha1.ClusterTopologyHA3Node {
		return nodes, nil
	}
	if len(nodes) < 3 {
		return nil, fmt.Errorf("ha_3_node requires at least 3 nodes (control plane)")
	}
	controls := 0
	workers := 0
	seenAddr := map[string]bool{}
	out := make([]fleetv1alpha1.FleetClusterJoinNodeSpec, 0, len(nodes))
	for i := range nodes {
		n := nodes[i]
		if n.Address == "" {
			return nil, fmt.Errorf("each node requires an address")
		}
		if seenAddr[n.Address] {
			return nil, fmt.Errorf("duplicate node address %s", n.Address)
		}
		seenAddr[n.Address] = true
		role := n.EffectiveRole()
		if role == fleetv1alpha1.NodeRoleWorker {
			workers++
			if n.Hostname == "" {
				n.Hostname = fmt.Sprintf("%s-w%d", clusterName, workers)
			}
			n.Role = fleetv1alpha1.NodeRoleWorker
		} else {
			controls++
			if n.Hostname == "" {
				n.Hostname = fmt.Sprintf("%s-%d", clusterName, controls)
			}
			n.Role = fleetv1alpha1.NodeRoleControl
		}
		out = append(out, n)
	}
	if controls != 3 {
		return nil, fmt.Errorf("ha_3_node requires exactly 3 control nodes (got %d)", controls)
	}
	return out, nil
}
