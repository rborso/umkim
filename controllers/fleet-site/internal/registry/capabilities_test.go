package registry

import "testing"

func TestCapabilitiesFromLabelsStorageAlias(t *testing.T) {
	caps := CapabilitiesFromLabels(map[string]string{
		"fleet.capability/storage.rwx": "true",
		"fleet.capability/ha_storage":  "true",
	})
	if !caps[CapStorageRWX] || !caps[CapHAStorage] {
		t.Fatalf("expected storage.rwx and ha_storage alias: %+v", caps)
	}
}

func TestApplyCapabilityPatchAliases(t *testing.T) {
	labels := map[string]string{}
	ApplyCapabilityPatch(labels, map[string]bool{
		CapStorageRWO: true,
		CapHATopology: true,
	})
	if labels["fleet.capability/ha_storage"] != "true" {
		t.Fatalf("ha_storage alias: %v", labels["fleet.capability/ha_storage"])
	}
	if labels["fleet.topology/ha"] != "true" {
		t.Fatalf("topology ha: %v", labels["fleet.topology/ha"])
	}
}

func TestClusterIngressEnabled(t *testing.T) {
	if !ClusterIngressEnabled(ClusterExposureInput{ExposureMode: "port_forward"}, "") {
		t.Fatal("port_forward should enable ingress")
	}
	if ClusterIngressEnabled(ClusterExposureInput{ExposureMode: "none"}, "port_forward") {
		t.Fatal("explicit none should not inherit site default")
	}
}
