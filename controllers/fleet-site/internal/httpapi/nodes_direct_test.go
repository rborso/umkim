package httpapi

import "testing"

func TestPatchSpokeKubeconfigServer(t *testing.T) {
	in := `server: https://127.0.0.1:6443`
	got := patchSpokeKubeconfigServer(in, "10.10.0.11")
	want := `server: https://10.10.0.11:6443`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFleetLabelsFromMC(t *testing.T) {
	got := fleetLabelsFromMC(map[string]string{
		"fleet.site":              "homelab",
		"fleet.capability/kvm":    "true",
		"cluster.open-cluster-management.io/clusterset": "default",
	})
	if got["fleet.site"] != "homelab" {
		t.Fatalf("fleet.site: %v", got)
	}
	if got["fleet.capability/kvm"] != "true" {
		t.Fatalf("capability: %v", got)
	}
	if _, ok := got["cluster.open-cluster-management.io/clusterset"]; ok {
		t.Fatalf("non-fleet label included: %v", got)
	}
}
