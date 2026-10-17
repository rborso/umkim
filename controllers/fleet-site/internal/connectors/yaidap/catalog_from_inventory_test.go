package yaidap

import "testing"

// A modern inventory has no catalog.json, so the controller must read the
// catalog from group_vars/all/all.yml + hypervisors.yml. Before this bridge the
// catalog came back empty, every network lookup missed, and physicalNetworkFor
// silently returned the lab fallback.
func TestCatalogFromInventory(t *testing.T) {
	data := map[string]string{
		"group_vars/all/all.yml": `
memory: 4096
cores: 2
storages:
  - name: local
    type: local
    shared: false
    path: /opt/local
networks:
  - network_name: mgmt_nw
    cidr: 192.168.1.0/24
    physical_network: mgmt_access_l1
    default_route: true
  - network_name: lab_nw
    cidr: 172.16.10.0/24
    physical_network: default_l2
`,
		"hypervisors.yml": `
hypervisors:
  hosts:
    virt01:
      ansible_host: 172.16.10.2
    virt02:
      ansible_host: 172.16.10.3
`,
	}

	cat := CatalogFromInventory(data)
	if len(cat.Networks) != 2 {
		t.Fatalf("networks = %d, want 2", len(cat.Networks))
	}
	if len(cat.Hypervisors) != 2 {
		t.Errorf("hypervisors = %v, want virt01+virt02", cat.Hypervisors)
	}
	if cat.Defaults.Memory != 4096 || cat.Defaults.Cores != 2 {
		t.Errorf("defaults = %d MiB / %d cores, want 4096 / 2", cat.Defaults.Memory, cat.Defaults.Cores)
	}

	// The behaviour the bug broke: a named network resolves to its physical
	// network, so YAIDAP can find the hypervisor uplink that offers it.
	if got := physicalNetworkFor(cat, "mgmt_nw"); got != "mgmt_access_l1" {
		t.Errorf("physicalNetworkFor(mgmt_nw) = %q, want mgmt_access_l1", got)
	}
	if got := physicalNetworkFor(cat, "lab_nw"); got != "default_l2" {
		t.Errorf("physicalNetworkFor(lab_nw) = %q, want default_l2", got)
	}
}
