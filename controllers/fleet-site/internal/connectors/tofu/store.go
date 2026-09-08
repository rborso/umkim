package tofu

import (
	"context"
	"encoding/json"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LoadBundle reads catalog + guests from the hub tofu store.
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

// Project renders skeleton tfvars files (Phase F scaffold).
func Project(catalog Catalog, guests GuestStore) (connectors.ProjectBundle, error) {
	_ = catalog
	files := map[string]string{}
	if len(guests) > 0 {
		raw, err := json.MarshalIndent(guests, "", "  ")
		if err != nil {
			return connectors.ProjectBundle{}, err
		}
		files["guests.auto.tfvars.json"] = string(raw) + "\n"
	}
	return connectors.ProjectBundle{Files: files}, nil
}

// ProjectFromStore loads and renders tofu inventory bundle.
func ProjectFromStore(ctx context.Context, c client.Client, ns, siteName string) (connectors.ProjectBundle, error) {
	catalog, guests, _, err := LoadBundle(ctx, c, ns, siteName)
	if err != nil {
		return connectors.ProjectBundle{}, err
	}
	return Project(catalog, guests)
}
