package yaidap

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuildHostFilesVM(t *testing.T) {
	catalog := Catalog{
		Networks: []NetworkRef{{Name: "lab_nw", PhysicalNetwork: "lab_access_l1", Zone: "lab"}},
		Storages: []StorageRef{{Name: "local_ssd"}},
		Defaults: GuestDefaults{OS: "rocky-10", Cores: 4, Memory: 8192, DiskGi: 50, Storage: "local_ssd", NetworkName: "lab_nw", Zone: "lab"},
	}
	intent := HostIntent{Kind: "vm", Name: "vm01", AnsibleHost: "172.16.10.60", Hypervisor: "virt04"}
	files, err := BuildHostFiles(map[string]string{}, intent, catalog)
	if err != nil {
		t.Fatalf("BuildHostFiles: %v", err)
	}
	guests := files["guests.yml"]
	if !strings.Contains(guests, "vm01:") || !strings.Contains(guests, "vm_location: virt04") || !strings.Contains(guests, "ansible_host: 172.16.10.60") {
		t.Fatalf("guests.yml missing entry:\n%s", guests)
	}
	hv := files["host_vars/vm01.yml"]
	for _, want := range []string{"memory: 8192", "cores: 4", "storage: local_ssd", "network_name: lab_nw", "physical_network: lab_access_l1", "size: 50", "ip: \"172.16.10.60\""} {
		if !strings.Contains(hv, want) {
			t.Errorf("host_vars missing %q:\n%s", want, hv)
		}
	}
}

func TestBuildHostFilesBareMetalDefaults(t *testing.T) {
	intent := HostIntent{Kind: "bare_metal", Name: "bm01", MACAddress: "aa:bb:cc:dd:ee:ff", PXE: true}
	files, err := BuildHostFiles(map[string]string{}, intent, Catalog{})
	if err != nil {
		t.Fatalf("BuildHostFiles: %v", err)
	}
	if !strings.Contains(files["bare_metals.yml"], "bm01:") {
		t.Fatalf("bare_metals.yml missing bm01:\n%s", files["bare_metals.yml"])
	}
	hv := files["host_vars/bm01.yml"]
	for _, want := range []string{"os: rocky-10", "install_os: false", "tempxe_discovery:", "enabled: true", "aa:bb:cc:dd:ee:ff"} {
		if !strings.Contains(hv, want) {
			t.Errorf("host_vars missing %q:\n%s", want, hv)
		}
	}
}

func TestBuildHostFilesHypervisorAndCluster(t *testing.T) {
	intent := HostIntent{Kind: "hypervisor", Name: "hv01", AnsibleHost: "172.16.10.9", Cluster: "lab-ha-0", NetworkName: "lab_nw", Zone: "lab"}
	files, err := BuildHostFiles(map[string]string{}, intent, Catalog{})
	if err != nil {
		t.Fatalf("BuildHostFiles: %v", err)
	}
	if !strings.Contains(files["hypervisors.yml"], "hv01:") {
		t.Fatalf("hypervisors.yml missing hv01:\n%s", files["hypervisors.yml"])
	}
	cluster := files["lab-ha-0.yml"]
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(cluster), &doc); err != nil {
		t.Fatalf("cluster yaml: %v", err)
	}
	group, _ := doc["lab-ha-0"].(map[string]interface{})
	hosts, _ := group["hosts"].([]interface{})
	if len(hosts) != 1 || hosts[0] != "hv01" {
		t.Fatalf("cluster hosts = %v", hosts)
	}
}

func TestBuildHostFilesRejectsBadInput(t *testing.T) {
	if _, err := BuildHostFiles(nil, HostIntent{Kind: "vm"}, Catalog{}); err == nil {
		t.Error("expected error for missing name")
	}
	if _, err := BuildHostFiles(nil, HostIntent{Kind: "router", Name: "x"}, Catalog{}); err == nil {
		t.Error("expected error for bad kind")
	}
	if _, err := BuildHostFiles(nil, HostIntent{Kind: "bare_metal", Name: "x"}, Catalog{}); err == nil {
		t.Error("expected error for bare_metal without ip/mac")
	}
}

func TestDeriveMAC(t *testing.T) {
	if got := deriveMAC("172.16.10.80"); got != "52:54:00:10:0a:50" {
		t.Errorf("deriveMAC = %q, want 52:54:00:10:0a:50", got)
	}
	if got := deriveMAC("bad"); got != "" {
		t.Errorf("deriveMAC(bad) = %q, want empty", got)
	}
}

func TestBuildHostFilesMergePreservesExisting(t *testing.T) {
	current := map[string]string{
		"guests.yml": "guests:\n  hosts:\n    vm00:\n      vm_location: virt01\n",
	}
	intent := HostIntent{Kind: "vm", Name: "vm01", Hypervisor: "least-utilized"}
	files, err := BuildHostFiles(current, intent, Catalog{})
	if err != nil {
		t.Fatalf("BuildHostFiles: %v", err)
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(files["guests.yml"]), &doc); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	hosts := doc["guests"].(map[string]interface{})["hosts"].(map[string]interface{})
	if _, ok := hosts["vm00"]; !ok {
		t.Fatal("existing guest vm00 dropped")
	}
	if _, ok := hosts["vm01"]; !ok {
		t.Fatal("new guest vm01 missing")
	}
}
