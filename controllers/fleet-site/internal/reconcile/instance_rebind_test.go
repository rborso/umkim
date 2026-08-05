package reconcile

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/dcm"
)

func TestRebindMode(t *testing.T) {
	tests := []struct {
		name   string
		rebind *fleetv1alpha1.FleetInstanceRestoreRebindSpec
		want   string
	}{
		{"nil", nil, ""},
		{"disabled", &fleetv1alpha1.FleetInstanceRestoreRebindSpec{Enabled: false}, ""},
		{"default enabled", &fleetv1alpha1.FleetInstanceRestoreRebindSpec{Enabled: true}, fleetv1alpha1.RebindModeSupersede},
		{"empty mode", &fleetv1alpha1.FleetInstanceRestoreRebindSpec{Enabled: true, Mode: ""}, fleetv1alpha1.RebindModeSupersede},
		{"supersede", &fleetv1alpha1.FleetInstanceRestoreRebindSpec{Enabled: true, Mode: "supersede"}, fleetv1alpha1.RebindModeSupersede},
		{"clone", &fleetv1alpha1.FleetInstanceRestoreRebindSpec{Enabled: true, Mode: "clone"}, fleetv1alpha1.RebindModeClone},
		{"unknown mode", &fleetv1alpha1.FleetInstanceRestoreRebindSpec{Enabled: true, Mode: "other"}, fleetv1alpha1.RebindModeSupersede},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rebindMode(tt.rebind); got != tt.want {
				t.Fatalf("rebindMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveSupersedeInstanceIDs(t *testing.T) {
	catalog := []dcm.CatalogItemInstance{
		{ID: "old-id", DisplayName: "gitea-br-test"},
		{ID: "new-id", DisplayName: "gitea-br-test"},
		{ID: "other", DisplayName: "whoami"},
	}
	got := resolveSupersedeInstanceIDs("backup-id", "gitea-br-test", catalog)
	want := []string{"backup-id", "new-id", "old-id"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestResolveRebindDisplayName(t *testing.T) {
	tests := []struct {
		mode, base, suffix, want string
	}{
		{fleetv1alpha1.RebindModeSupersede, "gitea-br-test", "", "gitea-br-test"},
		{fleetv1alpha1.RebindModeSupersede, "gitea-br-test", "-cross", "gitea-br-test"},
		{fleetv1alpha1.RebindModeClone, "gitea-br-test", "", "gitea-br-test-restored"},
		{fleetv1alpha1.RebindModeClone, "gitea-br-test", "-cross", "gitea-br-test-cross"},
	}
	for _, tt := range tests {
		t.Run(tt.mode+"_"+tt.suffix, func(t *testing.T) {
			if got := resolveRebindDisplayName(tt.mode, tt.base, tt.suffix); got != tt.want {
				t.Fatalf("resolveRebindDisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}
