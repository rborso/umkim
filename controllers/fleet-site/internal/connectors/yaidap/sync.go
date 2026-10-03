package yaidap

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
)

// SyncExtraVars returns ansible-playbook -e args for sync_inventory action.
func SyncExtraVars(bundle connectors.ProjectBundle) ([]string, error) {
	encoded, err := encodeBundleB64(bundle)
	if err != nil {
		return nil, err
	}
	return []string{
		"-e", "fleet_provision_action=" + connectors.InventoryActionSync,
		"-e", "fleet_provision_inventory_bundle_b64=" + encoded,
	}, nil
}

// ProvisionPreSyncExtraVars adds bundle push before a normal provision action.
func ProvisionPreSyncExtraVars(bundle connectors.ProjectBundle) ([]string, error) {
	encoded, err := encodeBundleB64(bundle)
	if err != nil {
		return nil, err
	}
	return []string{
		"-e", "fleet_provision_sync_from_store=true",
		"-e", "fleet_provision_inventory_bundle_b64=" + encoded,
	}, nil
}

func encodeBundleB64(bundle connectors.ProjectBundle) (string, error) {
	raw, err := EncodeBundleJSON(bundle)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString([]byte(raw)), nil
}

// ImportExtraVars returns ansible-playbook -e args for import_inventory action.
func ImportExtraVars(inventoryPath string) []string {
	return []string{
		"-e", "fleet_provision_action=" + connectors.InventoryActionImport,
		"-e", "fleet_provision_inventory_path=" + strings.TrimSpace(inventoryPath),
	}
}

// ImportPreviewExtraVars returns ansible-playbook -e args for import preview action.
func ImportPreviewExtraVars(inventoryPath string) []string {
	return []string{
		"-e", "fleet_provision_action=import_inventory_preview",
		"-e", "fleet_provision_inventory_path=" + strings.TrimSpace(inventoryPath),
	}
}

// DefaultImportInvocation maps import to connector playbook (no YAIDAP playbook).
func DefaultImportInvocation() (playbook string, tags []string) {
	return "site-provision-connector.yml", nil
}

// ValidateSyncBundle ensures projected files are present.
func ValidateSyncBundle(bundle connectors.ProjectBundle) error {
	if len(bundle.Files) == 0 {
		return fmt.Errorf("project bundle is empty")
	}
	for _, req := range []string{"guests.yml", "hypervisors.yml", "group_vars/all/all.yml"} {
		if _, ok := bundle.Files[req]; !ok {
			return fmt.Errorf("project bundle missing %s", req)
		}
	}
	return nil
}
