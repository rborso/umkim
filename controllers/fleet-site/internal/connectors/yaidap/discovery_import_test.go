package yaidap

import (
	"strings"
	"testing"
)

func TestPromoteDiscoveryHostVars(t *testing.T) {
	remote := map[string]string{
		"generated/host_vars/new-hv01.yml": `---
generated:
  tool: check_host_details
  generated_at: "2026-01-01T00:00:00Z"
legacy: false
os: rocky-10
uplinks:
  - mac_address: "aa:bb:cc:dd:ee:ff"
`,
	}
	promoted := PromoteDiscoveryFiles(nil, remote)
	got := promoted["host_vars/new-hv01.yml"]
	if strings.Contains(got, "generated:") {
		t.Fatalf("expected generated metadata stripped, got %q", got)
	}
	if !strings.Contains(got, "aa:bb:cc:dd:ee:ff") {
		t.Fatalf("expected mac preserved, got %q", got)
	}
}

func TestPromoteDiscoveryHostVarsPreferCanonical(t *testing.T) {
	remote := map[string]string{
		"host_vars/virt03.yml": `---
os: rocky-10
uplinks:
  - physical_network: lab_access_l1
`,
		"generated/host_vars/virt03.yml": `---
generated:
  tool: check_host_details
os: CHANGE_ME
uplinks:
  - physical_network: CHANGE_ME
`,
	}
	promoted := PromoteDiscoveryFiles(nil, remote)
	got := promoted["host_vars/virt03.yml"]
	if strings.Contains(got, "CHANGE_ME") {
		t.Fatalf("expected canonical host_vars preserved, got %q", got)
	}
	if !strings.Contains(got, "lab_access_l1") {
		t.Fatalf("expected synced host_vars content, got %q", got)
	}
}

func TestPromoteDiscoveryHypervisorsMerge(t *testing.T) {
	local := map[string]string{
		"file.hypervisors.yml": `hypervisors:
  hosts:
    existing:
      ansible_host: 10.0.0.1
`,
	}
	remote := map[string]string{
		"generated/hosts/new-hv01.yml": `hypervisors:
  hosts:
    new-hv01:
      ansible_host: 172.16.10.99
`,
	}
	promoted := PromoteDiscoveryFiles(local, remote)
	hv := promoted["hypervisors.yml"]
	if !strings.Contains(hv, "existing") {
		t.Fatalf("expected existing host preserved, got %q", hv)
	}
	if !strings.Contains(hv, "new-hv01") {
		t.Fatalf("expected new host merged, got %q", hv)
	}
	if !strings.Contains(hv, "172.16.10.99") {
		t.Fatalf("expected discovered ansible_host, got %q", hv)
	}
}

func TestPromoteDiscoveryHypervisorsFillEmptyAnsibleHost(t *testing.T) {
	local := map[string]string{
		"file.hypervisors.yml": `hypervisors:
  hosts:
    new-hv01: {}
`,
	}
	remote := map[string]string{
		"generated/hosts/new-hv01.yml": `hypervisors:
  hosts:
    new-hv01:
      ansible_host: 172.16.10.99
`,
	}
	promoted := PromoteDiscoveryFiles(local, remote)
	hv := promoted["hypervisors.yml"]
	if !strings.Contains(hv, "172.16.10.99") {
		t.Fatalf("expected ansible_host filled, got %q", hv)
	}
}

func TestPromoteDiscoveryHypervisorsPreserveExistingAnsibleHost(t *testing.T) {
	local := map[string]string{
		"file.hypervisors.yml": `hypervisors:
  hosts:
    new-hv01:
      ansible_host: 10.0.0.5
`,
	}
	remote := map[string]string{
		"generated/hosts/new-hv01.yml": `hypervisors:
  hosts:
    new-hv01:
      ansible_host: 172.16.10.99
`,
	}
	promoted := PromoteDiscoveryFiles(local, remote)
	hv := promoted["hypervisors.yml"]
	if strings.Contains(hv, "172.16.10.99") {
		t.Fatalf("expected existing ansible_host preserved, got %q", hv)
	}
	if !strings.Contains(hv, "10.0.0.5") {
		t.Fatalf("expected hub ansible_host kept, got %q", hv)
	}
}

func TestCompareImportFilesPromotesGenerated(t *testing.T) {
	local := map[string]string{
		"file.hypervisors.yml": `hypervisors:
  hosts:
    new-hv01: {}
`,
	}
	remote := map[string]string{
		"generated/host_vars/new-hv01.yml": `---
generated:
  tool: check_host_details
legacy: false
os: rocky-10
`,
		"generated/hosts/new-hv01.yml": `hypervisors:
  hosts:
    new-hv01:
      ansible_host: 172.16.10.99
`,
	}
	changes := CompareImportFiles(local, remote)
	byPath := map[string]string{}
	for _, ch := range changes {
		byPath[ch.Path] = ch.Status
	}
	if byPath["host_vars/new-hv01.yml"] != "added" {
		t.Fatalf("host_vars change=%+v", changes)
	}
	if byPath["hypervisors.yml"] != "modified" {
		t.Fatalf("hypervisors change=%+v", changes)
	}
}
