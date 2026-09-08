package connectors

import (
	"strings"
	"testing"
)

func TestFileKeyRoundTrip(t *testing.T) {
	paths := []string{
		"guests.yml",
		"hypervisors.yml",
		"lab-ha-1.yml",
		"host_vars/lab-single-1-0.yml",
		"group_vars/all/all.yml",
		"group_vars/all/common.yml",
		"group_vars/lab-ha-1.yml",
	}
	for _, p := range paths {
		key := FileKey(p)
		if strings.ContainsAny(key, "~") {
			t.Fatalf("key must be K8s-safe: %q", key)
		}
		got, ok := PathFromFileKey(key)
		if !ok || got != p {
			t.Fatalf("round-trip %q: key=%q got=%q ok=%v", p, key, got, ok)
		}
	}
}

func TestPathSynced(t *testing.T) {
	synced := []string{
		"guests.yml",
		"hypervisors.yml",
		"lab-ha-1.yml",
		"group_vars/all/all.yml",
		"group_vars/lab-ha-1.yml",
		"host_vars/foo.yml",
	}
	for _, p := range synced {
		if !pathSynced(p) {
			t.Fatalf("expected synced: %q", p)
		}
	}
	notSynced := []string{
		"clusters/lab-single-1.yml",
		"clusters.yml",
		"catalog.yml",
		"_meta/status.json",
		"controllers.yml",
	}
	for _, p := range notSynced {
		if pathSynced(p) {
			t.Fatalf("expected not synced: %q", p)
		}
	}
}

func TestRootInventoryGroupFile(t *testing.T) {
	if !IsRootInventoryGroupFile("lab-ha-1.yml") {
		t.Fatal("expected root group file")
	}
	if IsRootInventoryGroupFile("guests.yml") {
		t.Fatal("guests.yml is reserved")
	}
	if RootInventoryGroupName("lab-ha-1.yml") != "lab-ha-1" {
		t.Fatalf("name=%q", RootInventoryGroupName("lab-ha-1.yml"))
	}
}

func TestBuildSyncBundle(t *testing.T) {
	data := map[string]string{
		FileKey("guests.yml"):             "guests:\n  hosts: {}\n",
		FileKey("hypervisors.yml"):        "hypervisors:\n  hosts: {}\n",
		FileKey("group_vars/all/all.yml"): "storages: []\n",
		FileKey("host_vars/g1.yml"):       "memory: 4096\n",
		FileKey("lab-ha-1.yml"):           "lab-ha-1:\n  hosts:\n    g1:\n",
		FileKey("group_vars/lab-ha-1.yml"):"topology: single_node\n",
		FileKey("clusters/c1.yml"):        "topology: single_node\n",
	}
	bundle, err := BuildSyncBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bundle.Files["clusters/c1.yml"]; ok {
		t.Fatal("legacy clusters/ should not be in sync bundle")
	}
	if _, ok := bundle.Files["lab-ha-1.yml"]; !ok {
		t.Fatal("root cluster group should sync")
	}
	if len(bundle.Files) != 6 {
		t.Fatalf("expected 6 synced files, got %d", len(bundle.Files))
	}
}
