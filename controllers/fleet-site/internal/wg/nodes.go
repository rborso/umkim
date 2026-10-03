package wg

import (
	"encoding/json"
	"fmt"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

// NodeWG holds overlay identity for one cluster node.
type NodeWG struct {
	Hostname   string `json:"hostname"`
	Address    string `json:"address"`
	Role       string `json:"role,omitempty"`
	WGAddress  string `json:"wgAddress"`
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
}

// NodeWGFromJSON decodes nodes-wg.json from the bootstrap secret.
func NodeWGFromJSON(data []byte) ([]NodeWG, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var nodes []NodeWG
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

// NodeWGToJSON encodes node WG records for the bootstrap secret.
func NodeWGToJSON(nodes []NodeWG) ([]byte, error) {
	return json.Marshal(nodes)
}

// ConfigBlock renders overlay WireGuard config content for a node.
func ConfigBlock(priv, addr, hubPub, endpointHost, endpointPort, keepalive, subnet string) string {
	return fmt.Sprintf(`# UMKIM fleet cluster overlay
[Interface]
Address = %s
PrivateKey = %s

[Peer]
PublicKey = %s
Endpoint = %s:%s
AllowedIPs = %s
PersistentKeepalive = %s
`, addr, priv, hubPub, endpointHost, endpointPort, subnet, keepalive)
}

func nodeSpecsFromJoin(fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) []fleetv1alpha1.FleetClusterJoinNodeSpec {
	specs := fcj.Spec.Nodes
	if fcj.Spec.IsHA3Node() {
		if len(specs) >= 3 {
			return specs
		}
		host := fcj.Spec.BootstrapHost
		return []fleetv1alpha1.FleetClusterJoinNodeSpec{
			{Hostname: clusterName + "-1", Address: host, Role: fleetv1alpha1.NodeRoleControl},
			{Hostname: clusterName + "-2", Address: "", Role: fleetv1alpha1.NodeRoleControl},
			{Hostname: clusterName + "-3", Address: "", Role: fleetv1alpha1.NodeRoleControl},
		}
	}
	host := fcj.Spec.BootstrapHost
	hn := clusterName
	if len(specs) > 0 && specs[0].Hostname != "" {
		hn = specs[0].Hostname
	}
	if len(specs) > 0 && specs[0].Address != "" {
		host = specs[0].Address
	}
	if len(specs) > 0 {
		return specs
	}
	return []fleetv1alpha1.FleetClusterJoinNodeSpec{{Hostname: hn, Address: host, Role: fleetv1alpha1.NodeRoleControl}}
}

// AllocateClusterNodes assigns overlay IPs and keypairs for all nodes in a join spec.
func AllocateClusterNodes(
	fcj *fleetv1alpha1.FleetClusterJoin,
	clusterName string,
	used map[string]bool,
	poolStart, poolEnd int,
	existing []NodeWG,
) ([]NodeWG, error) {
	specs := nodeSpecsFromJoin(fcj, clusterName)

	existingByHost := map[string]NodeWG{}
	for _, n := range existing {
		existingByHost[n.Hostname] = n
	}

	out := make([]NodeWG, 0, len(specs))
	controlIdx := 0
	workerIdx := 0
	for _, spec := range specs {
		hn := spec.Hostname
		role := spec.EffectiveRole()
		if hn == "" {
			if role == fleetv1alpha1.NodeRoleWorker {
				workerIdx++
				hn = fmt.Sprintf("%s-w%d", clusterName, workerIdx)
			} else {
				controlIdx++
				hn = fmt.Sprintf("%s-%d", clusterName, controlIdx)
			}
		}
		if prev, ok := existingByHost[hn]; ok && prev.PrivateKey != "" && prev.PublicKey != "" && prev.WGAddress != "" {
			if prev.Role == "" {
				prev.Role = role
			}
			out = append(out, prev)
			used[HostIP(prev.WGAddress)] = true
			continue
		}
		addr, err := AllocateAddress(used, poolStart, poolEnd)
		if err != nil {
			return nil, err
		}
		used[HostIP(addr)] = true
		priv, pub, err := GenKeyPair()
		if err != nil {
			return nil, err
		}
		out = append(out, NodeWG{
			Hostname:   hn,
			Address:    spec.Address,
			Role:       role,
			WGAddress:  addr,
			PrivateKey: priv,
			PublicKey:  pub,
		})
	}
	return out, nil
}
