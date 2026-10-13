package httpapi

import "testing"

func TestParseVerifyOutput(t *testing.T) {
	raw := "host=virt01.lab.local\n" +
		"kernel=6.12.0\n" +
		"cpus=8\n" +
		"mem_mb=31968\n" +
		"libvirt_service=libvirtd\n" +
		"virsh_version=11.10.0\n" +
		"domains=1\n" +
		"pools=0\n" +
		"networks=0\n"
	res := parseVerifyOutput(raw)
	if res.Host != "virt01.lab.local" || res.LibvirtService != "libvirtd" || res.VirshVersion != "11.10.0" {
		t.Fatalf("parsed = %+v", res)
	}
	if !res.ok() {
		t.Fatal("expected ok for a host with libvirt + virsh")
	}
	if got := res.summary(); got != "verified (read-only): libvirt 11.10.0 (libvirtd), 1 domain(s), 8 vCPU, 31 GiB RAM" {
		t.Fatalf("summary = %q", got)
	}
}

func TestParseVerifyOutputNotOk(t *testing.T) {
	if parseVerifyOutput("host=x\n").ok() {
		t.Fatal("expected not ok when libvirt is missing")
	}
	if parseVerifyOutput("").ok() {
		t.Fatal("expected not ok for empty output")
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("a'b"); got != `'a'\''b'` {
		t.Fatalf("shellQuote = %q", got)
	}
}
