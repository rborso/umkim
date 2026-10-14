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
	hosts, _ := group["hosts"].(map[string]interface{})
	if len(hosts) != 1 {
		t.Fatalf("cluster hosts = %v", hosts)
	}
	if _, ok := hosts["hv01"]; !ok {
		t.Fatalf("cluster hosts missing hv01: %v", hosts)
	}
	// The generated group file must pass the same validator the API uses: the
	// map form is what Ansible and ParseInventoryGroupMembers require.
	if err := ValidateInventoryFile("lab-ha-0.yml", cluster); err != nil {
		t.Fatalf("generated cluster file does not validate: %v", err)
	}
	if members, err := ParseInventoryGroupMembers(cluster, "lab-ha-0"); err != nil {
		t.Fatalf("ParseInventoryGroupMembers: %v", err)
	} else if len(members) != 1 || members[0] != "hv01" {
		t.Fatalf("members = %v", members)
	}
}

func TestMergeClusterGroupHealsLegacyListForm(t *testing.T) {
	legacy := "lab-ha-0:\n  hosts:\n    - old-node\n"
	files, err := BuildHostFiles(
		map[string]string{"lab-ha-0.yml": legacy},
		HostIntent{Kind: "vm", Name: "new-node", AnsibleHost: "172.16.10.11", Cluster: "lab-ha-0"},
		Catalog{},
	)
	if err != nil {
		t.Fatalf("BuildHostFiles: %v", err)
	}
	members, err := ParseInventoryGroupMembers(files["lab-ha-0.yml"], "lab-ha-0")
	if err != nil {
		t.Fatalf("ParseInventoryGroupMembers: %v", err)
	}
	if strings.Join(members, ",") != "new-node,old-node" {
		t.Fatalf("members = %v (legacy member must be kept)", members)
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

func TestBuildHostFilesVMWithDisks(t *testing.T) {
	catalog := Catalog{
		Storages: []StorageRef{{Name: "local_ssd"}},
		Defaults: GuestDefaults{DiskGi: 40, Storage: "local_ssd"},
	}
	intent := HostIntent{
		Kind: "vm", Name: "vm02", AnsibleHost: "172.16.10.61", Hypervisor: "virt04",
		Disks: []HostDiskIntent{{SizeGi: 100}, {Name: "fast", SizeGi: 200, Storage: "local_ssd"}},
	}
	files, err := BuildHostFiles(map[string]string{}, intent, catalog)
	if err != nil {
		t.Fatalf("BuildHostFiles: %v", err)
	}
	hv := files["host_vars/vm02.yml"]
	for _, want := range []string{
		"  - name: bootdisk\n    disk: vda\n    bootdisk: true\n    device: 0\n",
		"    size: 40\n",
		"  - name: disk1\n    disk: vdb\n    device: 1\n",
		"    size: 100\n",
		"  - name: fast\n    disk: vdc\n    device: 2\n",
		"    size: 200\n",
	} {
		if !strings.Contains(hv, want) {
			t.Errorf("host_vars missing %q:\n%s", want, hv)
		}
	}
	// YAIDAP takes disks[0] as the boot device, so the boot disk must come first.
	if strings.Index(hv, "name: bootdisk") > strings.Index(hv, "name: disk1") {
		t.Errorf("boot disk must precede data disks:\n%s", hv)
	}
	// Data disks must not claim bootdisk.
	if strings.Contains(hv, "name: disk1\n    disk: vdb\n    bootdisk") {
		t.Errorf("data disk marked as boot:\n%s", hv)
	}
}

func TestBuildHostFilesRejectsBadDisks(t *testing.T) {
	if _, err := BuildHostFiles(map[string]string{}, HostIntent{Kind: "vm", Name: "x", Disks: []HostDiskIntent{{}}}, Catalog{}); err == nil {
		t.Fatal("want error for a disk with no size")
	}
	if _, err := BuildHostFiles(map[string]string{}, HostIntent{
		Kind: "bare_metal", Name: "x", AnsibleHost: "172.16.10.9", Disks: []HostDiskIntent{{SizeGi: 10}},
	}, Catalog{}); err == nil {
		t.Fatal("want error for disks on a non-vm kind")
	}
	many := make([]HostDiskIntent, maxGuestDisks+1)
	for i := range many {
		many[i] = HostDiskIntent{SizeGi: 10}
	}
	if _, err := BuildHostFiles(map[string]string{}, HostIntent{Kind: "vm", Name: "x", Disks: many}, Catalog{}); err == nil {
		t.Fatal("want error for too many disks")
	}
}

// Generated inventory must match the repo's 2-space YAML convention.
func TestBuildHostFilesUsesTwoSpaceIndent(t *testing.T) {
	files, err := BuildHostFiles(map[string]string{}, HostIntent{Kind: "vm", Name: "vm03", Hypervisor: "virt01"}, Catalog{})
	if err != nil {
		t.Fatalf("BuildHostFiles: %v", err)
	}
	if !strings.Contains(files["guests.yml"], "guests:\n  hosts:\n    vm03:") {
		t.Fatalf("guests.yml should nest with 2 spaces:\n%s", files["guests.yml"])
	}
}
