package provision

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
)

var inventoryImportRE = regexp.MustCompile(`FLEET_INVENTORY_IMPORT=(\{.*\})`)
var inventoryFilesRE = regexp.MustCompile(`(?s)FLEET_INVENTORY_FILES=(\{.*\})`)
var inventoryFilesB64RE = regexp.MustCompile(`FLEET_INVENTORY_FILES_B64=([A-Za-z0-9+/=]+)`)
var ansibleFoldRE = regexp.MustCompile(`\n\s+`)

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
	if match := inventoryFilesB64RE.FindStringSubmatch(logs); len(match) >= 2 {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(match[1]))
		if err != nil {
			return nil, false
		}
		var out map[string]string
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, false
		}
		return out, true
	}
	match := inventoryFilesRE.FindStringSubmatch(logs)
	if len(match) < 2 {
		if raw, ok := unwrapLegacyInventoryFilesJSON(logs); ok {
			return raw, true
		}
		return nil, false
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(unwrapFoldedJSON(match[1])), &out); err != nil {
		if raw, ok := unwrapLegacyInventoryFilesJSON(logs); ok {
			return raw, true
		}
		return nil, false
	}
	return out, true
}

func unwrapFoldedJSON(raw string) string {
	return ansibleFoldRE.ReplaceAllString(raw, "")
}

func unwrapLegacyInventoryFilesJSON(logs string) (map[string]string, bool) {
	idx := strings.Index(logs, "FLEET_INVENTORY_FILES=")
	if idx < 0 {
		return nil, false
	}
	end := strings.Index(logs[idx:], "'")
	if end < 0 {
		return nil, false
	}
	raw := logs[idx+len("FLEET_INVENTORY_FILES=") : idx+end]
	var out map[string]string
	if err := json.Unmarshal([]byte(unwrapFoldedJSON(raw)), &out); err != nil {
		return nil, false
	}
	return out, true
}

// InventoryJobSucceeded reports sync marker in job logs.
func InventoryJobSucceeded(logs string) bool {
	return strings.Contains(logs, "FLEET_INVENTORY_SYNCED=1")
}
