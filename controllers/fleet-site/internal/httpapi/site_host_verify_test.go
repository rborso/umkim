package httpapi

import (
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

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
	if !res.ok(fleetv1alpha1.SiteHostKindHypervisor) {
		t.Fatal("expected ok for a host with libvirt + virsh")
	}
	if got := res.summary(fleetv1alpha1.SiteHostKindHypervisor); got != "verified (read-only): libvirt 11.10.0 (libvirtd), 1 domain(s), 8 vCPU, 31 GiB RAM" {
		t.Fatalf("summary = %q", got)
	}
}

func TestParseVerifyOutputNotOk(t *testing.T) {
	if parseVerifyOutput("host=x\n").ok(fleetv1alpha1.SiteHostKindHypervisor) {
		t.Fatal("expected not ok when libvirt is missing")
	}
	if parseVerifyOutput("").ok(fleetv1alpha1.SiteHostKindHypervisor) {
		t.Fatal("expected not ok for empty output")
	}
}

// A workload node has no libvirt: it is verified through the K3s supervisor.
func TestParseVerifyOutputWorkload(t *testing.T) {
	raw := "host=lab-single-0.lab.local\n" +
		"kernel=6.12.0\n" +
		"cpus=8\n" +
		"mem_mb=31968\n" +
		"k3s_service=k3s\n" +
		"k3s_version=v1.36.5+k3s1\n" +
		"node_ready=True\n" +
		"node_ip=172.16.10.4\n" +
		"node_version=v1.36.5+k3s1\n" +
		"nodes=1\n"
	res := parseVerifyOutput(raw)
	if res.K3sService != "k3s" || res.NodeReady != "True" || res.NodeIP != "172.16.10.4" {
		t.Fatalf("parsed = %+v", res)
	}
	if !res.ok(fleetv1alpha1.SiteHostKindBareMetal) {
		t.Fatal("expected ok for a workload node running k3s and Ready")
	}
	if res.ok(fleetv1alpha1.SiteHostKindHypervisor) {
		t.Fatal("k3s output must not verify a hypervisor")
	}
	want := "verified (read-only): k3s v1.36.5+k3s1 (k3s), node Ready=True, 172.16.10.4, 1 node(s), 8 vCPU, 31 GiB RAM"
	if got := res.summary(fleetv1alpha1.SiteHostKindBareMetal); got != want {
		t.Fatalf("summary = %q", got)
	}
}

func TestWorkloadNotOkWhenNodeNotReady(t *testing.T) {
	if parseVerifyOutput("k3s_service=k3s\nnode_ready=False\n").ok(fleetv1alpha1.SiteHostKindBareMetal) {
		t.Fatal("expected not ok when the node is NotReady")
	}
	if parseVerifyOutput("k3s_service=k3s\n").ok(fleetv1alpha1.SiteHostKindBareMetal) {
		t.Fatal("expected not ok when the node has no readiness reported")
	}
	if got := parseVerifyOutput("host=x\n").summary(fleetv1alpha1.SiteHostKindBareMetal); got != "read-only check: k3s not responding" {
		t.Fatalf("summary = %q", got)
	}
	// A hypervisor's libvirt output must not satisfy a workload check.
	libvirt := "libvirt_service=libvirtd\nvirsh_version=11.10.0\n"
	if parseVerifyOutput(libvirt).ok(fleetv1alpha1.SiteHostKindBareMetal) {
		t.Fatal("libvirt output must not verify a workload node")
	}
	if !strings.Contains(parseVerifyOutput(libvirt).summary(fleetv1alpha1.SiteHostKindBareMetal), "k3s not responding") {
		t.Fatal("workload summary should name the missing control plane")
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("a'b"); got != `'a'\''b'` {
		t.Fatalf("shellQuote = %q", got)
	}
}
