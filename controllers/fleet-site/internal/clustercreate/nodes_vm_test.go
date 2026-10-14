package clustercreate

import (
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func vmNode(hostname, address string, vm *fleetv1alpha1.FleetClusterJoinNodeVM) fleetv1alpha1.FleetClusterJoinNodeSpec {
	return fleetv1alpha1.FleetClusterJoinNodeSpec{
		Hostname: hostname,
		Address:  address,
		Role:     fleetv1alpha1.NodeRoleControl,
		VM:       vm,
	}
}

func TestNormalizeNodesAcceptsVMBlock(t *testing.T) {
	nodes := []fleetv1alpha1.FleetClusterJoinNodeSpec{
		vmNode("lab-1", "172.16.10.101", &fleetv1alpha1.FleetClusterJoinNodeVM{Cores: 4, Memory: 6144, DiskGi: 50}),
		vmNode("lab-2", "172.16.10.102", &fleetv1alpha1.FleetClusterJoinNodeVM{}),
		vmNode("lab-3", "172.16.10.103", &fleetv1alpha1.FleetClusterJoinNodeVM{}),
	}
	out, err := NormalizeNodes("lab", fleetv1alpha1.ClusterTopologyHA3Node, "172.16.10.101", nodes)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || out[0].VM == nil || out[0].VM.Cores != 4 {
		t.Fatalf("vm spec not preserved: %+v", out)
	}
}

func TestNormalizeNodesRejectsPlaceholderVMAddress(t *testing.T) {
	nodes := []fleetv1alpha1.FleetClusterJoinNodeSpec{
		vmNode("lab-1", "198.51.100.21", &fleetv1alpha1.FleetClusterJoinNodeVM{}),
		vmNode("lab-2", "172.16.10.102", &fleetv1alpha1.FleetClusterJoinNodeVM{}),
		vmNode("lab-3", "172.16.10.103", &fleetv1alpha1.FleetClusterJoinNodeVM{}),
	}
	_, err := NormalizeNodes("lab", fleetv1alpha1.ClusterTopologyHA3Node, "198.51.100.21", nodes)
	if err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("want placeholder error, got %v", err)
	}
}

func TestNormalizeNodesAllowsPlaceholderWithoutVM(t *testing.T) {
	// A node without a vm spec keeps the old behavior (external/BM), so the
	// console's placeholder prefill is still accepted for hand-managed nodes.
	nodes := []fleetv1alpha1.FleetClusterJoinNodeSpec{
		{Hostname: "lab-1", Address: "198.51.100.21", Role: fleetv1alpha1.NodeRoleControl},
		{Hostname: "lab-2", Address: "198.51.100.22", Role: fleetv1alpha1.NodeRoleControl},
		{Hostname: "lab-3", Address: "198.51.100.23", Role: fleetv1alpha1.NodeRoleControl},
	}
	if _, err := NormalizeNodes("lab", fleetv1alpha1.ClusterTopologyHA3Node, "198.51.100.21", nodes); err != nil {
		t.Fatalf("placeholder without vm must be accepted: %v", err)
	}
}

func TestNormalizeNodesRejectsBadVMAddresses(t *testing.T) {
	for name, address := range map[string]string{
		"ipv6":       "fd00::1",
		"loopback":   "127.0.0.1",
		"link-local": "169.254.10.5",
		"not-ip":     "host.example",
	} {
		nodes := []fleetv1alpha1.FleetClusterJoinNodeSpec{
			vmNode("lab-1", address, &fleetv1alpha1.FleetClusterJoinNodeVM{}),
			vmNode("lab-2", "172.16.10.102", &fleetv1alpha1.FleetClusterJoinNodeVM{}),
			vmNode("lab-3", "172.16.10.103", &fleetv1alpha1.FleetClusterJoinNodeVM{}),
		}
		if _, err := NormalizeNodes("lab", fleetv1alpha1.ClusterTopologyHA3Node, address, nodes); err == nil {
			t.Fatalf("%s (%s): want error", name, address)
		}
	}
}

func TestNormalizeNodesRejectsNegativeVMResources(t *testing.T) {
	nodes := []fleetv1alpha1.FleetClusterJoinNodeSpec{
		vmNode("lab-1", "172.16.10.101", &fleetv1alpha1.FleetClusterJoinNodeVM{Cores: -1}),
		vmNode("lab-2", "172.16.10.102", &fleetv1alpha1.FleetClusterJoinNodeVM{}),
		vmNode("lab-3", "172.16.10.103", &fleetv1alpha1.FleetClusterJoinNodeVM{}),
	}
	if _, err := NormalizeNodes("lab", fleetv1alpha1.ClusterTopologyHA3Node, "172.16.10.101", nodes); err == nil {
		t.Fatal("negative cores must be rejected")
	}
}
