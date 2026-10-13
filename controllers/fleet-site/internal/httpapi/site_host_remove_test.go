package httpapi

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
