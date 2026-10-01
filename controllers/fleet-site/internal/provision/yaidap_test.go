package provision

import "testing"

func TestValidateProvider(t *testing.T) {
	if err := ValidateProvider("noop"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProvider("bogus"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRequiresConfirm(t *testing.T) {
	if !RequiresConfirm("install_bm") {
		t.Fatal("install_bm should require confirm")
	}
	if RequiresConfirm("discover") {
		t.Fatal("discover should not require confirm")
	}
	if !RequiresConfirm("discover_pxe") {
		t.Fatal("discover_pxe should require confirm")
	}
	if !RequiresConfirm("prepare_gpu_passthrough") {
		t.Fatal("prepare_gpu_passthrough should require confirm")
	}
	if RequiresConfirm("prepare_hypervisor") {
		t.Fatal("prepare_hypervisor should not require confirm")
	}
}

func TestDefaultYaidapInvocation(t *testing.T) {
	inv, err := DefaultYaidapInvocation("create_vm", false)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Playbook == "" || len(inv.Tags) == 0 {
		t.Fatalf("incomplete invocation: %+v", inv)
	}
	plan, err := DefaultYaidapInvocation("plan_vm", false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Playbook != "playbooks/plan-vm-placement.yml" || len(plan.Tags) != 1 || plan.Tags[0] != "plan" {
		t.Fatalf("unexpected plan invocation: %+v", plan)
	}
	bm, err := DefaultYaidapInvocation("install_bm", true)
	if err != nil {
		t.Fatal(err)
	}
	if bm.Playbook != "playbooks/install-bare-metal.yml" || len(bm.Tags) != 1 || bm.Tags[0] != "install_bm" {
		t.Fatalf("unexpected install_bm invocation: %+v", bm)
	}
	if bm.ConfirmE == "" {
		t.Fatal("expected tempxe_confirm extra")
	}
	if len(bm.ExtraArgs) != 2 || bm.ExtraArgs[0] != "-e" || bm.ExtraArgs[1] != "install_os=true" {
		t.Fatalf("expected install_os extra, got %+v", bm.ExtraArgs)
	}
	pxe, err := DefaultYaidapInvocation("discover_pxe", true)
	if err != nil {
		t.Fatal(err)
	}
	if pxe.Playbook != "playbooks/discover-pxe.yml" || pxe.Tags[0] != "discover_pxe" {
		t.Fatalf("unexpected discover_pxe invocation: %+v", pxe)
	}
	for _, arg := range pxe.ExtraArgs {
		if arg == "install_os=true" {
			t.Fatal("discover_pxe should not pass install_os")
		}
	}
	prep, err := DefaultYaidapInvocation("prepare_hypervisor", false)
	if err != nil {
		t.Fatal(err)
	}
	if prep.Playbook != "playbooks/configure-hypervisors.yml" || prep.Tags[0] != "prepare" {
		t.Fatalf("unexpected prepare_hypervisor: %+v", prep)
	}
	gpu, err := DefaultYaidapInvocation("prepare_hypervisor_gpu", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(gpu.Tags) != 2 || gpu.Tags[0] != "prepare" || gpu.Tags[1] != "prepare_gpu_passthrough" {
		t.Fatalf("unexpected prepare_hypervisor_gpu: %+v", gpu)
	}
}
