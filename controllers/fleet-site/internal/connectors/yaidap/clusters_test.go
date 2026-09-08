package yaidap

import (
	"strings"
	"testing"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
)

func TestSplitClustersYAML(t *testing.T) {
	raw := `clusters:
  lab-single-1:
    topology: single_node
    autoApply: true
    members: [lab-single-1-0]
  lab-ha-1:
    topology: ha_3_node
    autoApply: true
    controlPlaneVip: 172.16.10.160
    members: [cp0, cp1, cp2]
`
	files, err := SplitClustersYAML(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Fatalf("files=%v", files)
	}
	if !strings.Contains(files["group_vars/lab-single-1.yml"], "single_node") {
		t.Fatalf("single=%q", files["group_vars/lab-single-1.yml"])
	}
	if !strings.Contains(files["lab-single-1.yml"], "lab-single-1-0") {
		t.Fatalf("root=%q", files["lab-single-1.yml"])
	}
}

func TestLoadClustersFromData(t *testing.T) {
	data := map[string]string{
		connectors.FileKey("lab-single-1.yml"):            "lab-single-1:\n  hosts:\n    a:\n",
		connectors.FileKey("group_vars/lab-single-1.yml"): "topology: single_node\nautoApply: true\n",
		connectors.FileKey("guests.yml"):                  "guests:\n  hosts:\n    a:\n      ansible_host: 1.2.3.4\nexternal:\n  hosts: {}\n",
	}
	doc := LoadClustersFromData(data)
	if len(doc.Clusters) != 1 {
		t.Fatalf("clusters=%v", doc.Clusters)
	}
	spec, ok := doc.Clusters["lab-single-1"]
	if !ok || len(spec.Members) != 1 || spec.Members[0] != "a" {
		t.Fatalf("spec=%+v", spec)
	}
}

func TestValidateClusterGroupVars(t *testing.T) {
	guests := `guests:
  hosts:
    vm01:
      ansible_host: 1.2.3.4
external:
  hosts: {}
`
	content := "topology: single_node\n"
	if err := validateClusterGroupVars("group_vars/c1.yml", content, guests, []string{"vm01"}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateHypervisorHostVars(t *testing.T) {
	hvs := `hypervisors:
  hosts:
    virt01:
      ansible_host: 172.16.10.2
`
	if err := validateHostVarsYAML("host_vars/virt01.yml", "os: rocky-10\n", "", hvs); err != nil {
		t.Fatal(err)
	}
}

func TestParseInventoryGroupMembers(t *testing.T) {
	raw := `lab-ha-1:
  hosts:
    cp0:
    cp1:
`
	members, err := ParseInventoryGroupMembers(raw, "lab-ha-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 || members[0] != "cp0" {
		t.Fatalf("members=%v", members)
	}
}
