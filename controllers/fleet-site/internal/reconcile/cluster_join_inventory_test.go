package reconcile

import (
	"context"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/wg"
)

func testClusterJoinReconciler() *ClusterJoinReconciler {
	return &ClusterJoinReconciler{
		Reconciler: &Reconciler{
			Cfg: config.Config{
				HubWireguardIP:        "10.10.0.1",
				HubHostInventoryName:  "umkim-hub",
				SSHUser:               "linux",
				WireguardEndpointHost: "wg.example.com",
				WireguardEndpointPort: "51820",
			},
		},
	}
}

func TestBuildClusterInventoryINI_singleNodeUsesMgmtLAN(t *testing.T) {
	r := testClusterJoinReconciler()
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName:   "single-lab",
			SiteId:        "homelab",
			BootstrapHost: "198.51.100.92",
			Topology:      fleetv1alpha1.ClusterTopologySingleNode,
		},
	}
	nodes := []wg.NodeWG{{
		Hostname:  "single-lab",
		Address:   "198.51.100.92",
		WGAddress: "10.10.0.14/32",
	}}
	ini := r.buildClusterInventoryINI(context.Background(), fcj, "single-lab", nodes, false, "")
	assertInventoryLANNetworking(t, ini)
	if !strings.Contains(ini, "fleet_node_mgmt_address=198.51.100.92") {
		t.Fatal("missing fleet_node_mgmt_address on bootstrap vars")
	}
	if !strings.Contains(ini, "wireguard_address=10.10.0.14/32") {
		t.Fatal("missing overlay wireguard_address")
	}
	if strings.Contains(ini, "calico_ip_autodetection_method=interface=wg-umkim") {
		t.Fatal("calico must not use wg-umkim")
	}
}

func TestBuildClusterInventoryINI_haUsesMgmtLANPerNode(t *testing.T) {
	r := testClusterJoinReconciler()
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName:      "homelab-ha",
			SiteId:           "homelab",
			BootstrapHost:    "198.51.100.81",
			Topology:         fleetv1alpha1.ClusterTopologyHA3Node,
			ControlPlaneVip:  "198.51.100.84",
			Nodes: []fleetv1alpha1.FleetClusterJoinNodeSpec{
				{Hostname: "virt02-ha-cp1", Address: "198.51.100.81", Role: fleetv1alpha1.NodeRoleControl},
				{Hostname: "virt02-ha-cp2", Address: "198.51.100.82", Role: fleetv1alpha1.NodeRoleControl},
				{Hostname: "virt02-ha-cp3", Address: "198.51.100.83", Role: fleetv1alpha1.NodeRoleControl},
			},
		},
	}
	nodes := []wg.NodeWG{
		{Hostname: "virt02-ha-cp1", Address: "198.51.100.81", WGAddress: "10.10.0.10/32", Role: fleetv1alpha1.NodeRoleControl},
		{Hostname: "virt02-ha-cp2", Address: "198.51.100.82", WGAddress: "10.10.0.12/32", Role: fleetv1alpha1.NodeRoleControl},
		{Hostname: "virt02-ha-cp3", Address: "198.51.100.83", WGAddress: "10.10.0.13/32", Role: fleetv1alpha1.NodeRoleControl},
	}
	ini := r.buildClusterInventoryINI(context.Background(), fcj, "homelab-ha", nodes, false, "")
	assertInventoryLANNetworking(t, ini)
	for _, want := range []string{
		"fleet_node_mgmt_address=198.51.100.81",
		"fleet_node_mgmt_address=198.51.100.82",
		"fleet_node_mgmt_address=198.51.100.83",
		"k3s_cluster_join_server=https://198.51.100.84:6443",
	} {
		if !strings.Contains(ini, want) {
			t.Fatalf("missing %q in inventory", want)
		}
	}
	if !strings.Contains(ini, "[site_bootstrap:vars]") || !strings.Contains(ini, "calico_ip_autodetection_method=auto") {
		t.Fatal("site_bootstrap:vars must include calico autodetection for bootstrap playbooks")
	}
}

func TestBuildClusterInventoryINI_haHubRunnerKeepsMgmtOnNodes(t *testing.T) {
	r := testClusterJoinReconciler()
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName:     "homelab-ha",
			SiteId:          "homelab",
			BootstrapHost:   "198.51.100.81",
			Topology:        fleetv1alpha1.ClusterTopologyHA3Node,
			ControlPlaneVip: "198.51.100.84",
		},
	}
	nodes := []wg.NodeWG{
		{Hostname: "virt02-ha-cp1", Address: "198.51.100.81", WGAddress: "10.10.0.10/32", Role: fleetv1alpha1.NodeRoleControl},
	}
	ini := r.buildClusterInventoryINI(context.Background(), fcj, "homelab-ha", nodes, true, "")
	if !strings.Contains(ini, "virt02-ha-cp1 ansible_host=10.10.0.10") {
		t.Fatal("hub runner should SSH via overlay ansible_host")
	}
	if !strings.Contains(ini, "fleet_node_mgmt_address=198.51.100.81") {
		t.Fatal("mgmt address must stay on LAN for k3s")
	}
	assertInventoryLANNetworking(t, ini)
	if !strings.Contains(ini, "[site_bootstrap:vars]") || !strings.Contains(ini, "calico_ip_autodetection_method=auto") {
		t.Fatal("site_bootstrap:vars must include calico autodetection for bootstrap playbooks")
	}
}

func assertInventoryLANNetworking(t *testing.T, ini string) {
	t.Helper()
	for _, want := range []string{
		"calico_ip_autodetection_method=auto",
		"k3s_node_ip={{ fleet_node_mgmt_address | default(ansible_host) }}",
		"k3s_advertise_address={{ fleet_node_mgmt_address | default(ansible_host) }}",
		"fleet_cluster_wireguard_interface=wg-umkim",
		"wireguard_endpoint_host=wg.example.com",
		"wireguard_endpoint_port=51820",
	} {
		if !strings.Contains(ini, want) {
			t.Fatalf("missing %q in inventory", want)
		}
	}
}
