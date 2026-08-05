package reconcile

import (
	"sort"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/dcm"
)

const defaultCloneSuffix = "-restored"

// rebindMode returns the effective rebind mode. Default is supersede when rebind is enabled.
func rebindMode(rebind *fleetv1alpha1.FleetInstanceRestoreRebindSpec) string {
	if rebind == nil || !rebind.Enabled {
		return ""
	}
	mode := strings.TrimSpace(rebind.Mode)
	if mode == fleetv1alpha1.RebindModeClone {
		return fleetv1alpha1.RebindModeClone
	}
	return fleetv1alpha1.RebindModeSupersede
}

// resolveSupersedeInstanceIDs returns DCM instance IDs to remove before supersede rebind.
// Includes the backup instanceId and any catalog rows sharing the backup display name
// (e.g. after a prior restore created a new ID for the same workload).
func resolveSupersedeInstanceIDs(backupInstanceID, displayName string, catalog []dcm.CatalogItemInstance) []string {
	seen := map[string]struct{}{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		seen[id] = struct{}{}
	}
	add(backupInstanceID)
	for _, item := range catalog {
		if item.DisplayName == displayName {
			add(item.InstanceID())
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// resolveRebindDisplayName returns the DCM display name for a rebind operation.
func resolveRebindDisplayName(mode, baseName, suffix string) string {
	if mode == fleetv1alpha1.RebindModeClone {
		if suffix == "" {
			suffix = defaultCloneSuffix
		}
		return baseName + suffix
	}
	return baseName
}
