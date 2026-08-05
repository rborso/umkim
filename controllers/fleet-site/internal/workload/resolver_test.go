package workload

import (
	"testing"

	"github.com/rborso/umkim/controllers/fleet-site/internal/dcm"
)

func TestProviderToCluster(t *testing.T) {
	if got := ProviderToCluster("helm-lab-int01"); got != "lab-int01" {
		t.Fatalf("got %q", got)
	}
	if got := ProviderToCluster("k8s-lab-gpu03"); got != "lab-gpu03" {
		t.Fatalf("got %q", got)
	}
}

func TestClusterFromPlacement(t *testing.T) {
	targets := &dcm.PlacementTargetsResponse{
		Targets: []dcm.PlacementTarget{
			{Provider: "helm-lab-int01", Role: "primary", Status: "active"},
		},
	}
	if got := clusterFromPlacement(targets); got != "lab-int01" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyUserValuePatches(t *testing.T) {
	base := []dcm.UserValue{dcm.UserValueFromString("routing.cluster", "lab-int01"), dcm.UserValueFromString("metadata.name", "app")}
	patches := []dcm.UserValue{dcm.UserValueFromString("routing.cluster", "lab-gpu03")}
	out := ApplyUserValuePatches(base, patches)
	if dcm.UserValueString(out, "routing.cluster") != "lab-gpu03" {
		t.Fatalf("patch not applied: %+v", out)
	}
	if dcm.UserValueString(out, "metadata.name") != "app" {
		t.Fatalf("base value lost: %+v", out)
	}
}
