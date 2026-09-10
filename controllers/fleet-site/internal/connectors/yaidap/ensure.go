package yaidap

import (
	"context"
	"fmt"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcileStoreFromHosts is removed; inventory is edited via YAML files API.

// MarkSyncing records an in-flight push to the jumphost.
func MarkSyncing(ctx context.Context, c client.Client, ns, siteName string) error {
	return connectors.PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
		meta, err := decodeMeta(data["meta.json"])
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		meta.SyncPhase = "syncing"
		return connectors.InventoryPatch{Meta: &meta}, nil
	})
}

// FinalizeSyncFromJobLogs marks inventory synced when connector job output confirms push.
func FinalizeSyncFromJobLogs(ctx context.Context, c client.Client, ns, siteName, inventoryPath, logs string) error {
	if !containsInventorySynced(logs) {
		return nil
	}
	return MarkSynced(ctx, c, ns, siteName, inventoryPath)
}

func containsInventorySynced(logs string) bool {
	return strings.Contains(logs, "FLEET_INVENTORY_SYNCED=1")
}

// EnsureReadyForProvision validates inventory is present before YAIDAP provision actions.
func EnsureReadyForProvision(ctx context.Context, c client.Client, ns, siteName, actor string) error {
	_ = actor
	if err := MigrateLegacyStore(ctx, c, ns, siteName, "system/reconcile"); err != nil {
		return fmt.Errorf("migrate inventory: %w", err)
	}
	data, err := connectors.Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("inventory store missing; create guests.yml first")
	}
	if _, ok := connectors.GetInventoryFile(connectors.NormalizeMetaKeys(data), "guests.yml"); !ok {
		return fmt.Errorf("guests.yml missing in inventory store")
	}
	return nil
}
