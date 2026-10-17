package yaidap

import (
	"strings"
	"testing"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
)

// virt01 offers lab_nw on n0p0 and mgmt_nw on n0p1 — the shape that broke the
// guest render, which assumed n0p0 for every network.
const virt01HostVars = `
ansible_host: 172.16.10.2
os: rocky-10
provisioning_provider: libvirt
uplinks:
  - name: n0p0
    physical_network: lab_access_l1
    networks:
      - network_name: lab_nw
        bridge_name: n0p0_lab_nw
        ip: "172.16.10.2"
  - name: n0p1
    physical_network: mgmt_access_l1
    networks:
      - network_name: mgmt_nw
        bridge_name: n0p1_mgmt_nw
        ip: "192.168.1.110"
`

func TestResolveHypervisorUplink(t *testing.T) {
	if up, br := ResolveHypervisorUplink(virt01HostVars, "mgmt_nw"); up != "n0p1" || br != "n0p1_mgmt_nw" {
		t.Errorf("mgmt_nw resolved to %q/%q, want n0p1/n0p1_mgmt_nw", up, br)
	}
	if up, br := ResolveHypervisorUplink(virt01HostVars, "lab_nw"); up != "n0p0" || br != "n0p0_lab_nw" {
		t.Errorf("lab_nw resolved to %q/%q, want n0p0/n0p0_lab_nw", up, br)
	}
	if up, _ := ResolveHypervisorUplink(virt01HostVars, "nope_nw"); up != "" {
		t.Errorf("unknown network resolved to %q, want empty", up)
	}
}

// End to end through the renderer: a guest on mgmt_nw must come out attached to
// n0p1/n0p1_mgmt_nw with the physical network that uplink carries.
func TestGuestRenderUsesHypervisorUplink(t *testing.T) {
	current := map[string]string{
		"host_vars/virt01.yml": virt01HostVars,
	}
	catalog := Catalog{
		Networks: []NetworkRef{
			{Name: "mgmt_nw", PhysicalNetwork: "mgmt_access_l1"},
			{Name: "lab_nw", PhysicalNetwork: "default_l2"},
		},
	}
	files, err := BuildHostFiles(current, HostIntent{
		Kind:        "vm",
		Name:        "thintest0",
		AnsibleHost: "192.168.1.151",
		Hypervisor:  "virt01",
		NetworkName: "mgmt_nw",
		Zone:        "external",
		Cores:       6,
		Memory:      12288,
		DiskSizeGi:  80,
	}, catalog)
	if err != nil {
		t.Fatalf("BuildHostFiles: %v", err)
	}
	got := files["host_vars/thintest0.yml"]
	for _, want := range []string{
		"physical_network: mgmt_access_l1",
		"hypervisor_uplink: n0p1",
		"bridge_name: n0p1_mgmt_nw",
		`ip: "192.168.1.151"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered guest is missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "n0p0_mgmt_nw") {
		t.Errorf("rendered guest still assumes the first uplink:\n%s", got)
	}
}

// The FileKey-keyed read is easy to get wrong, so assert it directly.
func TestGetInventoryFileKeying(t *testing.T) {
	data := map[string]string{connectors.FileKey("hypervisors.yml"): "hypervisors:\n  hosts: {}\n"}
	if _, ok := connectors.GetInventoryFile(data, "hypervisors.yml"); !ok {
		t.Fatal("GetInventoryFile did not find a FileKey-keyed entry")
	}
}
