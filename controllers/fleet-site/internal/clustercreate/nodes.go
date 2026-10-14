package clustercreate

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

// nodeVMPlaceholderPrefixes are the documentation ranges (RFC 5737) that the
// console prefills. A VM guest cannot be created on one, so a node asking for a
// VM must carry the site's real address.
var nodeVMPlaceholderPrefixes = []netip.Prefix{
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
}

// validateNodeVMs checks the optional vm blocks before anything is created. The
// guest name is the node hostname and the guest address is the node address, so
// a placeholder or malformed address would only fail much later, during the VM
// create Job.
func validateNodeVMs(nodes []fleetv1alpha1.FleetClusterJoinNodeSpec) error {
	for i := range nodes {
		n := nodes[i]
		if n.VM == nil {
			continue
		}
		name := strings.TrimSpace(n.Hostname)
		if name == "" {
			name = fmt.Sprintf("node %d", i+1)
		}
		ip := net.ParseIP(strings.TrimSpace(n.Address))
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("node %q: vm requires an IPv4 address", name)
		}
		if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
			return fmt.Errorf("node %q: %s is not a usable VM address", name, ip)
		}
		if addr, ok := netip.AddrFromSlice(ip.To4()); ok {
			for _, prefix := range nodeVMPlaceholderPrefixes {
				if prefix.Contains(addr) {
					return fmt.Errorf("node %q: %s is a documentation placeholder; set the real site address before creating a VM", name, ip)
				}
			}
		}
		if n.VM.Cores < 0 || n.VM.Memory < 0 || n.VM.DiskGi < 0 || n.VM.DataDiskGi < 0 {
			return fmt.Errorf("node %q: vm resources must not be negative", name)
		}
	}
	return nil
}

// NormalizeNodes validates and fills node specs for cluster create.
func NormalizeNodes(clusterName, topology, bootstrapHost string, nodes []fleetv1alpha1.FleetClusterJoinNodeSpec) ([]fleetv1alpha1.FleetClusterJoinNodeSpec, error) {
	if err := validateNodeVMs(nodes); err != nil {
		return nil, err
	}
	return normalizeNodes(clusterName, topology, bootstrapHost, nodes)
}

func normalizeNodes(clusterName, topology, bootstrapHost string, nodes []fleetv1alpha1.FleetClusterJoinNodeSpec) ([]fleetv1alpha1.FleetClusterJoinNodeSpec, error) {
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
