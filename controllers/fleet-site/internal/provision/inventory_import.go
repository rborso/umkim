package provision

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
)

var inventoryImportRE = regexp.MustCompile(`FLEET_INVENTORY_IMPORT=(\{.*\})`)
var inventoryFilesRE = regexp.MustCompile(`FLEET_INVENTORY_FILES=(\{.*\})`)

// ParseInventoryImport extracts import JSON emitted by the connector playbook.
func ParseInventoryImport(logs string) (connectors.ImportResult, bool) {
	match := inventoryImportRE.FindStringSubmatch(logs)
	if len(match) < 2 {
		return connectors.ImportResult{}, false
	}
	var out connectors.ImportResult
	if err := json.Unmarshal([]byte(match[1]), &out); err != nil {
		return connectors.ImportResult{}, false
	}
	return out, true
}

// ParseInventoryFiles extracts file map from import preview job logs.
func ParseInventoryFiles(logs string) (map[string]string, bool) {
	match := inventoryFilesRE.FindStringSubmatch(logs)
	if len(match) < 2 {
		return nil, false
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(match[1]), &out); err != nil {
		return nil, false
	}
	return out, true
}

// InventoryJobSucceeded reports sync marker in job logs.
func InventoryJobSucceeded(logs string) bool {
	return strings.Contains(logs, "FLEET_INVENTORY_SYNCED=1")
}
