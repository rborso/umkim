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
	if bm.ConfirmE == "" {
		t.Fatal("expected tempxe_confirm extra")
	}
}
