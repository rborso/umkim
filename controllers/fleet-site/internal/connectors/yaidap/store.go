package yaidap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
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
	// catalog.json is the legacy key. A modern inventory carries
	// group_vars/all/all.yml + hypervisors.yml instead, and DecodeCatalog
	// returns an empty Catalog (not an error) for the absent key — which left
	// every network lookup taking its fallback. Bridge the modern files in.
	if len(catalog.Networks) == 0 {
		catalog = CatalogFromInventory(data)
	}
	guests, err := DecodeGuestStore(data["guests.json"])
	if err != nil {
		return Catalog{}, nil, meta, err
	}
	return catalog, guests, meta, nil
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
