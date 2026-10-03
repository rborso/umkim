package sitehosts

import "testing"

func TestValidateVmLocation(t *testing.T) {
	if err := ValidateVmLocation("vm", "least-utilized"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateVmLocation("vm", "virt01"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateVmLocation("vm", "not a host!"); err == nil {
		t.Fatal("expected error for invalid hostname")
	}
	if err := ValidateVmLocation("bare_metal", "anything"); err != nil {
		t.Fatal("bare metal should skip vm_location validation")
	}
}

func TestVmLocationIsScheduleToken(t *testing.T) {
	for _, tok := range []string{"", "least-utilized", "least_utilized", "auto", "LEAST-UTILIZED"} {
		if !VmLocationIsScheduleToken(tok) {
			t.Fatalf("expected schedule token %q", tok)
		}
	}
	if VmLocationIsScheduleToken("virt01") {
		t.Fatal("virt01 is not a schedule token")
	}
}
