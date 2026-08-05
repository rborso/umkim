package dcm

import (
	"encoding/json"
	"testing"
)

func testUserValue(path, value string) UserValue {
	raw, _ := json.Marshal(value)
	return UserValue{Path: path, Value: raw}
}

func TestUserValueString(t *testing.T) {
	vals := []UserValue{
		testUserValue("parent_cluster", "virt01"),
		testUserValue("metadata.name", "virt01-t1"),
	}
	if got := UserValueString(vals, "parent_cluster"); got != "virt01" {
		t.Fatalf("parent_cluster=%q", got)
	}
	if got := UserValueInt(vals, "control_plane.count", 1); got != 1 {
		t.Fatalf("default int=%d", got)
	}
}

func TestUserValueHelmField(t *testing.T) {
	vals := []UserValue{
		testUserValue("values", "model: my-model\nservice:\n  port: 9090\n"),
	}
	if got := UserValueHelmField(vals, "model"); got != "my-model" {
		t.Fatalf("model from yaml=%q", got)
	}
	if got := UserValueHelmInt(vals, 8080, "service", "port"); got != 9090 {
		t.Fatalf("port from yaml=%d", got)
	}
	vals = append(vals, testUserValue("values.model", "override"))
	if got := UserValueHelmField(vals, "model"); got != "override" {
		t.Fatalf("dotted path wins=%q", got)
	}
}

func TestIsClusterInstance(t *testing.T) {
	inst := CatalogItemInstance{
		Spec: CatalogItemInstanceSpec{
			CatalogItemID: "some-cluster-item",
			ServiceType:   "cluster",
		},
	}
	if !inst.IsClusterInstance() {
		t.Fatal("expected cluster instance")
	}
	nonCluster := CatalogItemInstance{
		Spec: CatalogItemInstanceSpec{
			CatalogItemID: "whoami",
			ServiceType:   "helm",
		},
	}
	if nonCluster.IsClusterInstance() {
		t.Fatal("expected non-cluster instance")
	}
}
