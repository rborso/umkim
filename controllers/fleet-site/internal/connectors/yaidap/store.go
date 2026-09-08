package yaidap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const provider = fleetv1alpha1.ProvisionProviderYaidap

// LoadBundle reads catalog + guests from the hub store.
func LoadBundle(ctx context.Context, c client.Client, ns, siteName string) (Catalog, GuestStore, connectors.InventoryMeta, error) {
	data, err := connectors.Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return Catalog{}, nil, connectors.InventoryMeta{}, err
	}
	meta, err := connectors.LoadMeta(ctx, c, ns, siteName, provider)
	if err != nil {
		return Catalog{}, nil, connectors.InventoryMeta{}, err
	}
	if data == nil {
		return Catalog{}, GuestStore{}, meta, nil
	}
	catalog, err := DecodeCatalog(data["catalog.json"])
	if err != nil {
		return Catalog{}, nil, meta, err
	}
	guests, err := DecodeGuestStore(data["guests.json"])
	if err != nil {
		return Catalog{}, nil, meta, err
	}
	return catalog, guests, meta, nil
}

// PutCatalog replaces catalog.json and appends a revision.
func PutCatalog(ctx context.Context, c client.Client, ns, siteName, actor string, catalog Catalog) error {
	if err := ValidateCatalog(&catalog); err != nil {
		return err
	}
	raw, err := json.Marshal(catalog)
	if err != nil {
		return err
	}
	var before json.RawMessage
	return connectors.PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
		before = json.RawMessage(data["catalog.json"])
		var log connectors.RevisionLog
		if r, err := decodeRevisionLog(data["revisions.json"]); err != nil {
			return connectors.InventoryPatch{}, err
		} else {
			log = r
		}
		rev := connectors.Revision{
			Actor:   actor,
			Scope:   connectors.RevisionScopeCatalog,
			Summary: "catalog updated",
			Before:  before,
			After:   json.RawMessage(raw),
		}
		log = connectors.AppendRevision(log, rev)
		meta, _ := decodeMeta(data["meta.json"])
		meta.RevisionHead = log.Items[0].ID
		meta.SyncPhase = "dirty"
		rawCopy := json.RawMessage(raw)
		return connectors.InventoryPatch{
			Meta:      &meta,
			Catalog:   &rawCopy,
			Revisions: &log,
		}, nil
	})
}

// PutGuest creates or replaces one guest and appends a revision.
func PutGuest(ctx context.Context, c client.Client, ns, siteName, guestName, actor string, spec GuestSpec) error {
	if err := ValidateGuestSpec(guestName, spec); err != nil {
		return err
	}
	return patchGuest(ctx, c, ns, siteName, actor, guestName, spec, "guest updated", false)
}

// DeleteGuest marks a guest absent or removes it from the store.
func DeleteGuest(ctx context.Context, c client.Client, ns, siteName, guestName, actor string, markAbsent bool) error {
	if err := ValidateGuestName(guestName); err != nil {
		return err
	}
	if markAbsent {
		return patchGuest(ctx, c, ns, siteName, actor, guestName, GuestSpec{Absent: true}, "guest marked absent", false)
	}
	return connectors.PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
		store, err := DecodeGuestStore(data["guests.json"])
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		beforeRaw, _ := json.Marshal(store[guestName])
		if _, ok := store[guestName]; !ok {
			return connectors.InventoryPatch{}, fmt.Errorf("guest %q not found", guestName)
		}
		delete(store, guestName)
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
		afterRaw, _ := json.Marshal(nil)
		log = connectors.AppendRevision(log, connectors.Revision{
			Actor:   actor,
			Scope:   connectors.RevisionScopeGuest,
			Summary: "guest deleted",
			Before:  beforeRaw,
			After:   afterRaw,
		})
		meta, _ := decodeMeta(data["meta.json"])
		meta.RevisionHead = log.Items[0].ID
		meta.SyncPhase = "dirty"
		rawMsg := json.RawMessage(raw)
		return connectors.InventoryPatch{
			Meta:      &meta,
			Guests:    &rawMsg,
			Revisions: &log,
		}, nil
	})
}

func patchGuest(ctx context.Context, c client.Client, ns, siteName, actor, guestName string, spec GuestSpec, summary string, merge bool) error {
	return connectors.PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
		store, err := DecodeGuestStore(data["guests.json"])
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		beforeRaw, _ := json.Marshal(store[guestName])
		if merge {
			existing := store[guestName]
			spec = mergeGuestSpec(existing, spec)
		}
		if err := ValidateGuestSpec(guestName, spec); err != nil {
			return connectors.InventoryPatch{}, err
		}
		store[guestName] = spec
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
		afterRaw := json.RawMessage(raw)
		log = connectors.AppendRevision(log, connectors.Revision{
			Actor:   actor,
			Scope:   connectors.RevisionScopeGuest,
			Summary: summary,
			Before:  beforeRaw,
			After:   afterRaw,
		})
		meta, _ := decodeMeta(data["meta.json"])
		meta.RevisionHead = log.Items[0].ID
		meta.SyncPhase = "dirty"
		rawMsg := json.RawMessage(raw)
		return connectors.InventoryPatch{
			Meta:      &meta,
			Guests:    &rawMsg,
			Revisions: &log,
		}, nil
	})
}

// PatchGuest merges fields into an existing guest.
func PatchGuest(ctx context.Context, c client.Client, ns, siteName, guestName, actor string, patch GuestSpec) error {
	if err := ValidateGuestName(guestName); err != nil {
		return err
	}
	return patchGuest(ctx, c, ns, siteName, actor, guestName, patch, "guest patched", true)
}

// EnsureGuestStub creates a minimal guest entry from a hub SiteHostRecord if missing.
func EnsureGuestStub(ctx context.Context, c client.Client, ns, siteName, actor string, rec fleetv1alpha1.SiteHostRecord) error {
	spec := GuestSpecFromHost(rec)
	return connectors.PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
		store, err := DecodeGuestStore(data["guests.json"])
		if err != nil {
			return connectors.InventoryPatch{}, err
		}
		if _, ok := store[rec.Name]; ok {
			return connectors.InventoryPatch{}, nil
		}
		store[rec.Name] = spec
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
		afterRaw := json.RawMessage(raw)
		log = connectors.AppendRevision(log, connectors.Revision{
			Actor:   actor,
			Scope:   connectors.RevisionScopeGuest,
			Summary: "guest stub from hub host",
			After:   afterRaw,
		})
		meta, _ := decodeMeta(data["meta.json"])
		meta.RevisionHead = log.Items[0].ID
		meta.SyncPhase = "dirty"
		rawMsg := json.RawMessage(raw)
		return connectors.InventoryPatch{
			Meta:      &meta,
			Guests:    &rawMsg,
			Revisions: &log,
		}, nil
	})
}

// GuestSpecFromHost builds a YAIDAP guest stub from fleet host intent.
func GuestSpecFromHost(rec fleetv1alpha1.SiteHostRecord) GuestSpec {
	kind := strings.TrimSpace(rec.Kind)
	spec := GuestSpec{
		Kind:        kind,
		AnsibleHost: strings.TrimSpace(rec.AnsibleHost),
		VmLocation:  strings.TrimSpace(rec.VmLocation),
	}
	if kind == fleetv1alpha1.SiteHostKindVM && spec.VmLocation == "" {
		spec.VmLocation = "least-utilized"
	}
	if kind == fleetv1alpha1.SiteHostKindBareMetal {
		spec.ProvisioningProvider = "bare_metal"
		spec.VmLocation = ""
	}
	if rec.Flavor != nil {
		if m := strings.TrimSpace(rec.Flavor["memory"]); m != "" {
			spec.Memory = atoiDefault(m, 0)
		}
		if c := strings.TrimSpace(rec.Flavor["cores"]); c != "" {
			spec.Cores = atoiDefault(c, 0)
		}
	}
	if os := strings.TrimSpace(rec.OSFamily); os != "" {
		spec.OS = os
	}
	return spec
}

func mergeGuestSpec(existing, patch GuestSpec) GuestSpec {
	out := existing
	if v := strings.TrimSpace(patch.Kind); v != "" {
		out.Kind = v
	}
	if v := strings.TrimSpace(patch.AnsibleHost); v != "" {
		out.AnsibleHost = v
	}
	if v := strings.TrimSpace(patch.VmLocation); v != "" {
		out.VmLocation = v
	}
	if v := strings.TrimSpace(patch.OS); v != "" {
		out.OS = v
	}
	if patch.Memory > 0 {
		out.Memory = patch.Memory
	}
	if patch.Cores > 0 {
		out.Cores = patch.Cores
	}
	if patch.Disks != nil {
		out.Disks = patch.Disks
	}
	if patch.Uplinks != nil {
		out.Uplinks = patch.Uplinks
	}
	if patch.Legacy != nil {
		out.Legacy = patch.Legacy
	}
	if patch.ConfigureFirewalld != nil {
		out.ConfigureFirewalld = patch.ConfigureFirewalld
	}
	if patch.ProvisioningProvider != "" {
		out.ProvisioningProvider = patch.ProvisioningProvider
	}
	out.Absent = patch.Absent
	return out
}

func decodeMeta(raw string) (connectors.InventoryMeta, error) {
	if strings.TrimSpace(raw) == "" {
		return connectors.InventoryMeta{Provider: provider, SchemaVersion: SchemaVersion, SyncPhase: "pending"}, nil
	}
	var meta connectors.InventoryMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return connectors.InventoryMeta{}, fmt.Errorf("invalid meta.json: %w", err)
	}
	if meta.Provider == "" {
		meta.Provider = provider
	}
	if meta.SchemaVersion == 0 {
		meta.SchemaVersion = SchemaVersion
	}
	return meta, nil
}

func decodeRevisionLog(raw string) (connectors.RevisionLog, error) {
	if strings.TrimSpace(raw) == "" {
		return connectors.RevisionLog{}, nil
	}
	var log connectors.RevisionLog
	if err := json.Unmarshal([]byte(raw), &log); err != nil {
		return connectors.RevisionLog{}, fmt.Errorf("invalid revisions.json: %w", err)
	}
	return log, nil
}

func atoiDefault(s string, def int) int {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
