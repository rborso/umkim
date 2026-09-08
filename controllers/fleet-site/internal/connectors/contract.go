package connectors

import (
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

// InventoryAction identifies connector inventory jobs (push sync / pull import).
const (
	InventoryActionSync   = "sync_inventory"
	InventoryActionImport = "import_inventory"
)

// InventoryMeta is stored in meta.json for a provider inventory ConfigMap.
type InventoryMeta struct {
	Provider       string `json:"provider"`
	SchemaVersion  int    `json:"schemaVersion"`
	LastSyncedAt   string `json:"lastSyncedAt,omitempty"`
	SyncPhase      string `json:"syncPhase,omitempty"`
	InventoryPath  string `json:"inventoryPath,omitempty"`
	RevisionHead   string `json:"revisionHead,omitempty"`
	LastImportAt   string `json:"lastImportAt,omitempty"`
	LastImportNote string `json:"lastImportNote,omitempty"`
}

// InventoryEnvelope is returned by GET /provision/inventory.
type InventoryEnvelope struct {
	Meta      InventoryMeta       `json:"meta"`
	Catalog   interface{}         `json:"catalog,omitempty"`
	Guests    interface{}         `json:"guests,omitempty"`
	Files     []InventoryFileInfo `json:"files,omitempty"`
	Revisions []Revision          `json:"revisions,omitempty"`
}

// ProjectBundle is rendered inventory written to the jumphost on sync.
type ProjectBundle struct {
	Files map[string]string `json:"files"`
}

// ImportResult is parsed stdout from import_inventory connector action.
type ImportResult struct {
	Catalog map[string]interface{}            `json:"catalog,omitempty"`
	Guests  map[string]map[string]interface{} `json:"guests,omitempty"`
	Hosts   []fleetv1alpha1.ProvisionHandoffHost `json:"hosts,omitempty"`
}
