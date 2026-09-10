package yaidap

import (
	"strings"
	"testing"
)

func TestInventoryPlaceholderWarningsHostVars(t *testing.T) {
	content := `uplinks:
  - name: n0p0
    physical_network: CHANGE_ME
    networks:
      - network_name: CHANGE_ME
        ip: 172.16.10.4
`
	warnings := InventoryPlaceholderWarnings("host_vars/virt03.yml", content)
	if len(warnings) != 2 {
		t.Fatalf("warnings=%v", warnings)
	}
	if !strings.Contains(warnings[0], "physical_network") {
		t.Fatalf("expected physical_network warning, got %q", warnings[0])
	}
	if !strings.Contains(warnings[1], "network_name") {
		t.Fatalf("expected network_name warning, got %q", warnings[1])
	}
}

func TestInventoryPlaceholderWarningsCleanFile(t *testing.T) {
	warnings := InventoryPlaceholderWarnings("host_vars/virt01.yml", "os: rocky-10\n")
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}

func TestCompareImportFilesPlaceholderWarnings(t *testing.T) {
	local := map[string]string{
		"file.host_vars.virt03.yml": "os: rocky-10\n",
	}
	remote := map[string]string{
		"host_vars/virt03.yml": "uplinks:\n  - physical_network: CHANGE_ME\n",
	}
	changes := CompareImportFiles(local, remote)
	if len(changes) != 1 {
		t.Fatalf("changes=%+v", changes)
	}
	if len(changes[0].Warnings) == 0 {
		t.Fatal("expected placeholder warnings on modified import preview item")
	}
}
