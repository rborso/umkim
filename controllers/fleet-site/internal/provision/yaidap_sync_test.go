package provision

import (
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestYaidapSyncExtraVars(t *testing.T) {
	extra := YaidapSyncExtraVars(fleetv1alpha1.SiteHostRecord{
		Kind:        fleetv1alpha1.SiteHostKindVM,
		VmLocation:  "least-utilized",
		AnsibleHost: "172.16.10.161",
		OSFamily:    "rocky-10",
		Flavor:      map[string]string{"memory": "8192", "cores": "4"},
	})
	if len(extra) == 0 {
		t.Fatal("expected sync extra vars")
	}
	joined := strings.Join(extra, " ")
	for _, want := range []string{
		"fleet_provision_sync_host=true",
		"fleet_provision_disk_gi=40",
		"fleet_provision_vm_location=least-utilized",
		"fleet_provision_ansible_host=172.16.10.161",
		"fleet_provision_memory=8192",
		"fleet_provision_cores=4",
		"fleet_provision_os=rocky-10",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
}

func TestParseResolvedVmLocation(t *testing.T) {
	logs := "TASK [Emit resolved vm_location marker for hub] ***\nok: [jumphost] => {\n    \"msg\": \"FLEET_YAIDAP_VM_LOCATION=virt02\"\n}\n"
	if got := ParseResolvedVmLocation(logs); got != "virt02" {
		t.Fatalf("got %q", got)
	}
	if ParseResolvedVmLocation("FLEET_YAIDAP_VM_LOCATION=least-utilized") != "" {
		t.Fatal("schedule token should be ignored")
	}
}

func TestActionSyncsYaidapInventory(t *testing.T) {
	if !ActionSyncsYaidapInventory("plan_vm") {
		t.Fatal("plan_vm should sync inventory")
	}
	if !ActionSyncsYaidapInventory("install_bm") {
		t.Fatal("install_bm should sync inventory")
	}
	if !ActionSyncsYaidapInventory("discover") {
		t.Fatal("discover should sync inventory")
	}
	if !ActionSyncsYaidapInventory("discover_pxe") {
		t.Fatal("discover_pxe should sync inventory")
	}
}
