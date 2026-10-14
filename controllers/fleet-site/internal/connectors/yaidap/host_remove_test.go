package yaidap

import (
	"strings"
	"testing"
)

const guestsSample = `guests:
  hosts:
    vm-a:
      ansible_host: 10.0.0.1
      vm_location: virt01
    vm-b:
      ansible_host: 10.0.0.2
      vm_location: virt02
    vm-c:
      ansible_host: 10.0.0.3
      vm_location: virt02
`

func TestRemoveHostBlockKeepsOtherHosts(t *testing.T) {
	out, ok := removeHostBlock(guestsSample, "vm-b")
	if !ok {
		t.Fatal("expected the host to be removed")
	}
	if strings.Contains(out, "vm-b") || strings.Contains(out, "10.0.0.2") {
		t.Fatalf("vm-b survived:\n%s", out)
	}
	for _, keep := range []string{"vm-a", "10.0.0.1", "vm-c", "10.0.0.3", "guests:", "  hosts:"} {
		if !strings.Contains(out, keep) {
			t.Fatalf("removal dropped %q:\n%s", keep, out)
		}
	}
}

func TestRemoveHostBlockFirstAndLast(t *testing.T) {
	out, _ := removeHostBlock(guestsSample, "vm-a")
	if strings.Contains(out, "vm-a") || !strings.Contains(out, "vm-b") || !strings.Contains(out, "vm-c") {
		t.Fatalf("first-entry removal wrong:\n%s", out)
	}
	out, _ = removeHostBlock(guestsSample, "vm-c")
	if strings.Contains(out, "vm-c") || !strings.Contains(out, "vm-a") || !strings.Contains(out, "vm-b") {
		t.Fatalf("last-entry removal wrong:\n%s", out)
	}
}

func TestRemoveHostBlockAbsent(t *testing.T) {
	out, ok := removeHostBlock(guestsSample, "nope")
	if ok || out != guestsSample {
		t.Fatalf("absent host changed the document (ok=%v)", ok)
	}
}

// guests.yml written by an older yaml.Marshal call nests with 4 spaces; removal
// must not depend on a fixed indent width.
const guestsSampleFourSpace = `guests:
    hosts:
        vm-a:
            ansible_host: 10.0.0.1
            vm_location: virt01
        vm-b:
            ansible_host: 10.0.0.2
            vm_location: virt02
`

func TestRemoveHostBlockFourSpaceIndent(t *testing.T) {
	out, ok := removeHostBlock(guestsSampleFourSpace, "vm-a")
	if !ok {
		t.Fatal("expected the host to be removed")
	}
	if strings.Contains(out, "vm-a") || strings.Contains(out, "10.0.0.1") {
		t.Fatalf("vm-a survived:\n%s", out)
	}
	for _, keep := range []string{"guests:", "    hosts:", "vm-b", "10.0.0.2", "vm_location: virt02"} {
		if !strings.Contains(out, keep) {
			t.Fatalf("removal dropped %q:\n%s", keep, out)
		}
	}
}

const clusterGroupSample = `lab-ha-0:
  hosts:
    lab-ha-0-1: {}
    lab-ha-0-2: {}
    lab-ha-0-3: {}
`

func TestPruneClusterGroupKeepsRemainingMembers(t *testing.T) {
	updated, drop, changed, err := pruneClusterGroup(clusterGroupSample, "lab-ha-0", []string{"lab-ha-0-2"})
	if err != nil || !changed || drop {
		t.Fatalf("changed=%v drop=%v err=%v", changed, drop, err)
	}
	if strings.Contains(updated, "lab-ha-0-2") {
		t.Fatalf("removed member survived:\n%s", updated)
	}
	for _, keep := range []string{"lab-ha-0-1", "lab-ha-0-3"} {
		if !strings.Contains(updated, keep) {
			t.Fatalf("kept member %q missing:\n%s", keep, updated)
		}
	}
	// The result must still parse as a root group file.
	if members, err := ParseInventoryGroupMembers(updated, "lab-ha-0"); err != nil || len(members) != 2 {
		t.Fatalf("members=%v err=%v", members, err)
	}
}

func TestPruneClusterGroupDropsEmptyFile(t *testing.T) {
	_, drop, changed, err := pruneClusterGroup(
		clusterGroupSample, "lab-ha-0", []string{"lab-ha-0-1", "lab-ha-0-2", "lab-ha-0-3"},
	)
	if err != nil || !changed || !drop {
		t.Fatalf("changed=%v drop=%v err=%v", changed, drop, err)
	}
}

func TestPruneClusterGroupNoOp(t *testing.T) {
	if _, _, changed, err := pruneClusterGroup(clusterGroupSample, "lab-ha-0", []string{"other"}); err != nil || changed {
		t.Fatalf("unknown host must be a no-op: changed=%v err=%v", changed, err)
	}
	if _, _, changed, err := pruneClusterGroup("other-group:\n  hosts:\n    x: {}\n", "lab-ha-0", []string{"x"}); err != nil || changed {
		t.Fatalf("other group must be a no-op: changed=%v err=%v", changed, err)
	}
}
