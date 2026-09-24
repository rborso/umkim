package yaidap

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplyImport merges pulled inventory and appends an import revision.
func ApplyImport(ctx context.Context, c client.Client, ns, siteName, actor string, pulled connectors.ImportResult, merge bool) error {
	catalog, guests, meta, err := LoadBundle(ctx, c, ns, siteName)
	if err != nil {
		return err
	}
	mergedCatalog, mergedGuests, err := MergeImport(catalog, guests, pulled, merge)
	if err != nil {
		return err
	}
	catalogRaw, err := json.Marshal(mergedCatalog)
	if err != nil {
		return err
	}
	guestsRaw, err := json.Marshal(mergedGuests)
	if err != nil {
		return err
	}
	return connectors.PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
		var log connectors.RevisionLog
		if r, err := decodeRevisionLog(data["revisions.json"]); err != nil {
			return connectors.InventoryPatch{}, err
		} else {
			log = r
		}
		beforeCatalog := json.RawMessage(data["catalog.json"])
		beforeGuests := json.RawMessage(data["guests.json"])
		log = connectors.AppendRevision(log, connectors.Revision{
			Actor:   actor,
			Scope:   connectors.RevisionScopeImport,
			Summary: "import from jumphost",
			Before:  beforeGuests,
			After:   json.RawMessage(guestsRaw),
		})
		_ = beforeCatalog
		meta.LastImportAt = time.Now().UTC().Format(time.RFC3339)
		meta.LastImportNote = fmt.Sprintf("guests=%d", len(mergedGuests))
		meta.RevisionHead = log.Items[0].ID
		meta.SyncPhase = "synced"
		catalogMsg := json.RawMessage(catalogRaw)
		guestsMsg := json.RawMessage(guestsRaw)
		return connectors.InventoryPatch{
			Meta:      &meta,
			Catalog:   &catalogMsg,
			Guests:    &guestsMsg,
			Revisions: &log,
		}, nil
	})
}

// MarkImportFailed records an import apply error so GET inventory is not a silent sync.
func MarkImportFailed(ctx context.Context, c client.Client, ns, siteName string, importErr error) error {
	if importErr == nil {
		return nil
	}
	return connectors.PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
		meta, err := connectors.LoadMetaFromData(data, provider)
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		meta.SyncPhase = "failed"
		meta.LastImportNote = importErr.Error()
		return connectors.InventoryPatch{Meta: &meta}, nil
	})
}

// MarkSynced updates meta after successful push sync.
func MarkSynced(ctx context.Context, c client.Client, ns, siteName, inventoryPath string) error {
	return connectors.PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
		meta, err := connectors.LoadMetaFromData(data, provider)
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		meta.LastSyncedAt = time.Now().UTC().Format(time.RFC3339)
		meta.SyncPhase = "synced"
		if inventoryPath != "" {
			meta.InventoryPath = inventoryPath
		}
		data = connectors.NormalizeMetaKeys(data)
		var log connectors.RevisionLog
		if r, err := decodeRevisionLog(data[connectors.MetaKeyRevisions()]); err != nil {
			return connectors.InventoryPatch{}, err
		} else {
			log = r
		}
		log = connectors.AppendRevision(log, connectors.Revision{
			Actor:   "system",
			Scope:   connectors.RevisionScopeSync,
			Summary: "pushed inventory to jumphost",
		})
		meta.RevisionHead = log.Items[0].ID
		return connectors.InventoryPatch{
			Meta:      &meta,
			Revisions: &log,
		}, nil
	})
}

// ProjectFromStore builds jumphost sync bundle from hub YAML files (with legacy JSON fallback).
func ProjectFromStore(ctx context.Context, c client.Client, ns, siteName string) (connectors.ProjectBundle, error) {
	data, err := connectors.Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return connectors.ProjectBundle{}, err
	}
	if data == nil {
		return connectors.ProjectBundle{}, fmt.Errorf("inventory store is empty")
	}
	data = connectors.NormalizeMetaKeys(data)
	if _, ok := connectors.GetInventoryFile(data, "guests.yml"); ok {
		bundle, err := connectors.BuildSyncBundle(data)
		if err != nil {
			return connectors.ProjectBundle{}, err
		}
		bareMetalsRaw, _ := connectors.GetInventoryFile(data, "bare_metals.yml")
		composed, err := ComposeGuestsYAMLForSync(bundle.Files["guests.yml"], bareMetalsRaw)
		if err != nil {
			return connectors.ProjectBundle{}, err
		}
		bundle.Files["guests.yml"] = composed
		return bundle, ValidateSyncBundle(bundle)
	}
	// Legacy JSON store fallback.
	catalog, guests, _, err := LoadBundle(ctx, c, ns, siteName)
	if err != nil {
		return connectors.ProjectBundle{}, err
	}
	bundle, err := Project(catalog, guests)
	if err != nil {
		return connectors.ProjectBundle{}, err
	}
	return bundle, ValidateSyncBundle(bundle)
}
