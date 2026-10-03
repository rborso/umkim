package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInventoryMgmtIPs(t *testing.T) {
	dir := t.TempDir()
	ini := `[fleet_cluster_control]
cp1 ansible_host=172.16.10.101
cp2 ansible_host=172.16.10.102

[fleet_cluster_workers]
worker ansible_host=172.16.10.140

[fleet_cluster_nodes:vars]
foo=bar

[hub]
hub ansible_host=10.10.0.1
`
	path := filepath.Join(dir, "inventory.ini")
	if err := os.WriteFile(path, []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := inventoryMgmtIPs(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"172.16.10.101", "172.16.10.102", "172.16.10.140"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
