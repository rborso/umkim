package yaidap

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestListInventoryHostsIncludesHypervisors(t *testing.T) {
	site := "homelab"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName(site, fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: "fleet-console",
		},
		Data: map[string]string{
			"file.guests.yml": `guests:
  hosts:
    lab-single-1-0:
      vm_location: virt02
      ansible_host: 192.168.1.101
`,
			"file.hypervisors.yml": `hypervisors:
  hosts:
    virt03:
      ansible_host: 172.16.10.4
`,
			"_meta.status.json": `{}`,
		},
	}
	c := fake.NewClientBuilder().WithObjects(cm).Build()
	hosts, err := ListInventoryHosts(context.Background(), c, "fleet-console", site)
	if err != nil {
		t.Fatal(err)
	}
	var virt03 *InventoryHostView
	for i := range hosts {
		if hosts[i].Name == "virt03" {
			virt03 = &hosts[i]
			break
		}
	}
	if virt03 == nil {
		t.Fatal("expected virt03 in host list")
	}
	if virt03.Kind != fleetv1alpha1.SiteHostKindHypervisor {
		t.Fatalf("expected hypervisor kind, got %q", virt03.Kind)
	}
	if virt03.AnsibleHost != "172.16.10.4" {
		t.Fatalf("unexpected ansible_host %q", virt03.AnsibleHost)
	}
}

func TestListInventoryHostsExternalBareMetal(t *testing.T) {
	site := "homelab"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName(site, fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: "fleet-console",
		},
		Data: map[string]string{
			"file.guests.yml": `guests:
  hosts: {}
`,
			"file.bare_metals.yml": `bare_metals:
  hosts:
    bm01:
      ansible_host: 203.0.113.11
`,
			"file.hypervisors.yml": `hypervisors:
  hosts: {}
`,
			"_meta.status.json": `{}`,
		},
	}
	c := fake.NewClientBuilder().WithObjects(cm).Build()
	hosts, err := ListInventoryHosts(context.Background(), c, "fleet-console", site)
	if err != nil {
		t.Fatal(err)
	}
	var bm01 *InventoryHostView
	for i := range hosts {
		if hosts[i].Name == "bm01" {
			bm01 = &hosts[i]
			break
		}
	}
	if bm01 == nil {
		t.Fatal("expected bm01 in host list")
	}
	if bm01.Kind != fleetv1alpha1.SiteHostKindBareMetal {
		t.Fatalf("expected bare_metal kind, got %q", bm01.Kind)
	}
	if !bm01.External {
		t.Fatal("expected external=true")
	}
}

func TestListInventoryHostsClusterFromFleetClusterJoin(t *testing.T) {
	site := "homelab"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName(site, fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: "fleet-console",
		},
		Data: map[string]string{
			"file.guests.yml": `guests:
  hosts:
    lab-ha-0-cp1:
      ansible_host: 172.16.10.101
    lab-ha-0-w1:
      ansible_host: 172.16.10.140
`,
			"file.lab-ha-0.yml": `lab-ha-0:
  hosts:
    lab-ha-0-cp1:
`,
			"file.group_vars/lab-ha-0.yml": `topology: ha_3_node
controlPlaneVip: 172.16.10.100
`,
			"_meta.status.json": `{}`,
		},
	}
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "lab-ha-0", Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			SiteId:      site,
			ClusterName: "lab-ha-0",
			Topology:    fleetv1alpha1.ClusterTopologyHA3Node,
			Nodes: []fleetv1alpha1.FleetClusterJoinNodeSpec{
				{Hostname: "lab-ha-0-cp1", Address: "172.16.10.101", Role: fleetv1alpha1.NodeRoleControl},
				{Hostname: "lab-ha-0-w1", Address: "172.16.10.140", Role: fleetv1alpha1.NodeRoleWorker},
			},
		},
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			Phase:   fleetv1alpha1.JoinPhaseReady,
			Message: "Capability discovery complete",
		},
	}
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm, fcj).Build()
	hosts, err := ListInventoryHosts(context.Background(), c, "fleet-console", site)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]InventoryHostView{}
	for _, h := range hosts {
		byName[h.Name] = h
	}
	if len(byName["lab-ha-0-w1"].Clusters) != 1 || byName["lab-ha-0-w1"].Clusters[0] != "lab-ha-0" {
		t.Fatalf("worker clusters=%v", byName["lab-ha-0-w1"].Clusters)
	}
	if len(byName["lab-ha-0-cp1"].Clusters) != 1 || byName["lab-ha-0-cp1"].Clusters[0] != "lab-ha-0" {
		t.Fatalf("control clusters=%v", byName["lab-ha-0-cp1"].Clusters)
	}
	w := byName["lab-ha-0-w1"]
	if w.Status.Cluster == nil || w.Status.Cluster.Phase != fleetv1alpha1.SiteHostClusterPhaseReady {
		t.Fatalf("worker status.cluster=%+v", w.Status.Cluster)
	}
	if w.Status.Cluster.Mode != fleetv1alpha1.SiteHostClusterModeAddWorker {
		t.Fatalf("worker mode=%q", w.Status.Cluster.Mode)
	}
}
