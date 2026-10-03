package yaidap

import (
	"strings"
	"testing"
)

func TestCompareImportFilesUnchanged(t *testing.T) {
	local := map[string]string{
		"file.guests.yml": "guests:\n  hosts:\n    vm01:\n      ansible_host: 1.2.3.4\n",
	}
	remote := map[string]string{
		"guests.yml": "guests:\n  hosts:\n    vm01:\n      ansible_host: 1.2.3.4\n",
	}
	changes := CompareImportFiles(local, remote)
	if len(changes) != 1 || changes[0].Status != "unchanged" {
		t.Fatalf("changes=%+v", changes)
	}
}

func TestCompareImportFilesModified(t *testing.T) {
	local := map[string]string{
		"file.guests.yml": "guests:\n  hosts:\n    vm01:\n      ansible_host: 1.2.3.4\n",
	}
	remote := map[string]string{
		"guests.yml": "guests:\n  hosts:\n    vm01:\n      ansible_host: 5.6.7.8\n",
	}
	changes := CompareImportFiles(local, remote)
	if len(changes) != 1 || changes[0].Status != "modified" {
		t.Fatalf("changes=%+v", changes)
	}
	if !strings.Contains(changes[0].Diff, "-") || !strings.Contains(changes[0].Diff, "+") {
		t.Fatalf("diff=%q", changes[0].Diff)
	}
}

func TestApplyImportFilesPreservesHubOnly(t *testing.T) {
	local := map[string]string{
		"file.guests.yml":    "old\n",
		"file.lab-ha-1.yml":            "lab-ha-1:\n  hosts:\n    a:\n    b:\n    c:\n",
		"file.group_vars.lab-ha-1.yml": "topology: ha_3_node\ncontrolPlaneVip: 10.0.0.1\n",
		"_meta.status.json":  `{"guests":{}}`,
	}
	remote := map[string]string{
		"guests.yml": "new\n",
	}
	out := ApplyImportFiles(local, remote, []string{"guests.yml"})
	if out["guests.yml"] != "new\n" {
		t.Fatalf("guests=%q", out["guests.yml"])
	}
	if len(out) != 1 {
		t.Fatalf("unexpected keys in put map: %+v", out)
	}
}
