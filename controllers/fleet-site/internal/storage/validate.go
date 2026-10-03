package storage

import (
	"fmt"
	"net"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

const nfsDelegateInventoryHost = "fleet_nfs_delegate"

// NFSDelegateInventoryHost is the Ansible inventory name for remote export mkdir.
func NFSDelegateInventoryHost() string {
	return nfsDelegateInventoryHost
}

// ClusterNodeIPs returns management and WireGuard IPs for NFS locality checks.
func ClusterNodeIPs(fcj *fleetv1alpha1.FleetClusterJoin) []string {
	seen := map[string]bool{}
	add := func(ip string) {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			return
		}
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		ip = strings.TrimSuffix(ip, "/32")
		ip = strings.TrimSuffix(ip, "/24")
		if i := strings.Index(ip, "/"); i > 0 {
			ip = ip[:i]
		}
		if ip != "" && !seen[ip] {
			seen[ip] = true
		}
	}
	if fcj.Spec.BootstrapHost != "" {
		add(fcj.Spec.BootstrapHost)
	}
	for _, n := range fcj.Spec.Nodes {
		add(n.Address)
	}
	for _, n := range fcj.Status.Nodes {
		add(n.Address)
		if n.WireguardAddress != "" {
			add(n.WireguardAddress)
		}
	}
	if fcj.Status.WireguardAddress != "" {
		add(fcj.Status.WireguardAddress)
	}
	out := make([]string, 0, len(seen))
	for ip := range seen {
		out = append(out, ip)
	}
	return out
}

func serverMatchesClusterNode(server string, nodeIPs []string) bool {
	server = strings.TrimSpace(server)
	if server == "" {
		return false
	}
	if host, _, err := net.SplitHostPort(server); err == nil {
		server = host
	}
	for _, ip := range nodeIPs {
		if ip == server {
			return true
		}
	}
	return false
}

// ValidateNFSSpec checks nfs_external / nfs_local storage intent for create/configure.
func ValidateNFSSpec(st *fleetv1alpha1.ClusterStorageSpec, fcj *fleetv1alpha1.FleetClusterJoin) error {
	return validateNFSSpecFields(st, fcj)
}

func validateNFSSpecFields(st *fleetv1alpha1.ClusterStorageSpec, fcj *fleetv1alpha1.FleetClusterJoin) error {
	if st == nil || !st.NFSEnabled() {
		return nil
	}
	if st.NFSLocalEnabled() {
		topo := ""
		if fcj != nil {
			topo = fcj.Spec.Topology
		}
		if topo != "" && topo != fleetv1alpha1.ClusterTopologySingleNode {
			return fmt.Errorf("nfs_local is only supported for single_node topology")
		}
		clusterPath := ""
		if st.NFS != nil {
			clusterPath = strings.TrimSpace(st.NFS.ClusterPath)
		}
		if clusterPath == "" && fcj != nil {
			clusterPath = fcj.Spec.EffectiveClusterName(fcj.Name)
		}
		if clusterPath == "" {
			return fmt.Errorf("nfs_local requires storage.nfs.clusterPath or cluster name")
		}
		return nil
	}
	nfs := st.NFS
	if nfs == nil || strings.TrimSpace(nfs.Server) == "" {
		return fmt.Errorf("nfs_external requires storage.nfs.server")
	}
	clusterPath := strings.TrimSpace(nfs.ClusterPath)
	if clusterPath == "" && fcj != nil {
		clusterPath = fcj.Spec.EffectiveClusterName(fcj.Name)
	}
	if clusterPath == "" {
		return fmt.Errorf("nfs_external requires storage.nfs.clusterPath or cluster name")
	}
	return nil
}

// ValidateLonghornSpec checks longhorn storage intent for create.
func ValidateLonghornSpec(st *fleetv1alpha1.ClusterStorageSpec, topology string) error {
	if st == nil || !st.LonghornEnabled() {
		return nil
	}
	if topology != fleetv1alpha1.ClusterTopologyHA3Node {
		return fmt.Errorf("longhorn storage is only supported for ha_3_node topology")
	}
	dev := st.EffectiveLonghornDevice()
	// Empty device = directory mode on root FS (defaultDataPath). Dedicated disk must be /dev/*.
	if dev != "" && !strings.HasPrefix(dev, "/dev/") {
		return fmt.Errorf("storage.longhorn.device must be empty (root directory) or a block device path (e.g. /dev/vdb)")
	}
	return nil
}

// ValidateControlPlaneVip checks HA control-plane VIP for create.
func ValidateControlPlaneVip(vip string, topology string, nodeAddresses []string) error {
	if topology != fleetv1alpha1.ClusterTopologyHA3Node {
		return nil
	}
	vip = strings.TrimSpace(vip)
	if vip == "" {
		return fmt.Errorf("ha_3_node requires controlPlaneVip")
	}
	ip := net.ParseIP(vip)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("controlPlaneVip must be a valid IPv4 address")
	}
	for _, addr := range nodeAddresses {
		if strings.TrimSpace(addr) == vip {
			return fmt.Errorf("controlPlaneVip must not equal a node address")
		}
	}
	return nil
}

// NFSSpecChanged reports whether server or cluster export path changed (migration not supported).
func NFSSpecChanged(prev, next *fleetv1alpha1.ClusterStorageSpec, clusterName string) bool {
	if prev == nil || next == nil || next.NFS == nil || !next.NFSEnabled() {
		return false
	}
	if prev.NFS == nil || !prev.NFSEnabled() {
		return false
	}
	prevPath := prev.NFS.ClusterPath
	if prevPath == "" {
		prevPath = clusterName
	}
	nextPath := next.NFS.ClusterPath
	if nextPath == "" {
		nextPath = clusterName
	}
	return prev.NFS.Server != next.NFS.Server || prevPath != nextPath
}
