package yaidap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitehosts"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcileStoreFromHosts ensures connector guests exist and match hub SiteHostRecords.
func ReconcileStoreFromHosts(ctx context.Context, c client.Client, ns, siteName, actor string) error {
	hosts, err := sitehosts.List(ctx, c, ns, siteName)
	if err != nil {
		return err
	}
	for _, rec := range hosts {
		if err := reconcileGuestFromHost(ctx, c, ns, siteName, actor, rec); err != nil {
			return err
		}
	}
	return nil
}

func reconcileGuestFromHost(ctx context.Context, c client.Client, ns, siteName, actor string, rec fleetv1alpha1.SiteHostRecord) error {
	spec := GuestSpecFromHost(rec)
	return connectors.PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
		store, err := DecodeGuestStore(data["guests.json"])
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		if store == nil {
			store = GuestStore{}
		}
		beforeRaw, _ := json.Marshal(store[rec.Name])
		merged := mergeGuestSpec(store[rec.Name], spec)
		if err := ValidateGuestSpec(rec.Name, merged); err != nil {
			return connectors.InventoryPatch{}, err
		}
		if jsonEqual(store[rec.Name], merged) && strings.TrimSpace(data["meta.json"]) != "" {
			return connectors.InventoryPatch{}, nil
		}
		store[rec.Name] = merged
		raw, err := json.Marshal(store)
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		var log connectors.RevisionLog
		if r, err := decodeRevisionLog(data["revisions.json"]); err != nil {
			return connectors.InventoryPatch{}, err
		} else {
			log = r
		}
		if !bytesEqualJSON(beforeRaw, raw) {
			log = connectors.AppendRevision(log, connectors.Revision{
				Actor:   actor,
				Scope:   connectors.RevisionScopeGuest,
				Summary: "reconciled from hub host",
				Before:  beforeRaw,
				After:   json.RawMessage(raw),
			})
		}
		meta, err := decodeMeta(data["meta.json"])
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		if len(log.Items) > 0 {
			meta.RevisionHead = log.Items[0].ID
		}
		if meta.SyncPhase != "syncing" {
			meta.SyncPhase = "dirty"
		}
		rawMsg := json.RawMessage(raw)
		return connectors.InventoryPatch{
			Meta:      &meta,
			Guests:    &rawMsg,
			Revisions: &log,
		}, nil
	})
}

func bytesEqualJSON(before json.RawMessage, after []byte) bool {
	if len(before) == 0 && len(after) == 0 {
		return true
	}
	return string(before) == string(after)
}

func jsonEqual(a GuestSpec, b GuestSpec) bool {
	ra, _ := json.Marshal(a)
	rb, _ := json.Marshal(b)
	return string(ra) == string(rb)
}

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
