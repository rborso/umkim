package provision

import (
	"testing"
)

func TestParseYaidapResultCreateVM(t *testing.T) {
	logs := `TASK [emit YAIDAP_RESULT integration line] ***
ok: [localhost] => {
    "msg": "YAIDAP_RESULT={\"schema\":\"yaidap.integration.v1\",\"action\":\"create_vm\",\"subject\":\"vm01\",\"phase\":\"ready\",\"ok\":true,\"message\":\"VM created and SSH reachable\",\"vm_location\":\"virt02\",\"handoff\":{\"hostname\":\"vm01\",\"ansible_host\":\"172.16.10.21\",\"ansible_user\":\"linux\",\"os_family\":\"rocky-10\"},\"placement\":{\"assignments\":[{\"guest\":\"vm01\",\"hypervisor\":\"virt02\",\"memory_mib\":8192,\"source\":\"schedule\"}],\"unplaced\":[]},\"warnings\":[],\"errors\":[]}"
}
`
	result, ok := ParseYaidapResult(logs)
	if !ok || result == nil {
		t.Fatal("expected parse success")
	}
	if result.Action != "create_vm" || result.VmLocation != "virt02" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.ConcreteVmLocation() != "virt02" {
		t.Fatalf("vm_location=%q", result.VmLocation)
	}
	host := result.ToHandoffHost("homelab")
	if host == nil || host.AnsibleHost != "172.16.10.21" || host.OSFamily != "rocky-10" {
		t.Fatalf("handoff=%+v", host)
	}
	summary := FormatPlacementSummary(result.Placement)
	if summary != "placement: vm01→virt02" {
		t.Fatalf("summary=%q", summary)
	}
}

func TestParseYaidapResultUsesLastLine(t *testing.T) {
	logs := `YAIDAP_RESULT={"schema":"yaidap.integration.v1","action":"plan_vm","subject":"vm01","phase":"planned","ok":true,"message":"dry-run","placement":{"assignments":[{"guest":"vm01","hypervisor":"virt01","memory_mib":4096,"source":"schedule"}],"unplaced":[]}}
YAIDAP_RESULT={"schema":"yaidap.integration.v1","action":"plan_vm","subject":"vm02","phase":"planned","ok":true,"message":"final","placement":{"assignments":[{"guest":"vm02","hypervisor":"virt02","memory_mib":4096,"source":"schedule"}],"unplaced":[]}}
`
	result, ok := ParseYaidapResult(logs)
	if !ok || result.Subject != "vm02" || result.Message != "final" {
		t.Fatalf("result=%+v ok=%v", result, ok)
	}
}

func TestParseYaidapResultRejectsScheduleToken(t *testing.T) {
	result := &IntegrationResult{VmLocation: "least-utilized"}
	if result.ConcreteVmLocation() != "" {
		t.Fatal("expected empty for schedule token")
	}
}

func TestParseYaidapResultInvalid(t *testing.T) {
	if _, ok := ParseYaidapResult("no marker"); ok {
		t.Fatal("expected false")
	}
	if _, ok := ParseYaidapResult(`YAIDAP_RESULT={"schema":"other.v0"}`); ok {
		t.Fatal("expected false for wrong schema")
	}
}

func TestActionUsesConnectionLocal(t *testing.T) {
	if !ActionUsesConnectionLocal("discover_pxe") {
		t.Fatal("discover_pxe should use connection local")
	}
	if ActionUsesConnectionLocal("install_bm") {
		t.Fatal("install_bm must not force connection local on the whole playbook")
	}
}

func TestYaidapIntegrationExtraVars(t *testing.T) {
	base := YaidapIntegrationExtraVars("create_vm", false)
	if len(base) != 2 || base[0] != "-e" || base[1] != "yaidap_emit_result=true" {
		t.Fatalf("base=%v", base)
	}
	recreate := YaidapIntegrationExtraVars("create_vm", true)
	if len(recreate) != 4 {
		t.Fatalf("recreate=%v", recreate)
	}
	if YaidapIntegrationExtraVars("status", false) != nil {
		t.Fatal("status should not emit integration vars")
	}
}
