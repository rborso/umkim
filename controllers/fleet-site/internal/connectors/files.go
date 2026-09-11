package connectors

import (
	"fmt"
	"path"
	"strings"
)

const (
	metaKeyPrefix = "_meta."
	fileKeyPrefix = "file."

	metaKeyMeta          = "_meta.meta.json"
	metaKeyRevisions     = "_meta.revisions.json"
	metaKeyStatus        = "_meta.status.json"
	metaKeyImportPreview = "_meta.import_preview.json"

	// Legacy keys migrated on read.
	legacyMetaKey      = "meta.json"
	legacyRevisionsKey = "revisions.json"
	legacyCatalogKey   = "catalog.json"
	legacyGuestsKey    = "guests.json"
)

// InventoryFileInfo describes one inventory file in the hub store.
type InventoryFileInfo struct {
	Path   string `json:"path"`
	Synced bool   `json:"synced"`
	Size   int    `json:"size"`
}

// ReservedRootInventoryFiles are root group files with YAIDAP-specific schema.
var ReservedRootInventoryFiles = map[string]bool{
	"guests.yml":      true,
	"hypervisors.yml": true,
}

// HubCanonicalInventoryFiles are hub-only inventory paths (editable, not synced as-is).
var HubCanonicalInventoryFiles = map[string]bool{
	"bare_metals.yml": true,
}

// JumphostOnlyRootFiles are never stored or synced from the hub inventory store.
var JumphostOnlyRootFiles = map[string]bool{
	"controllers.yml": true,
	"routers.yml":     true,
}

func pathSynced(p string) bool {
	p = NormalizeInventoryPath(p)
	if HubCanonicalInventoryFiles[p] {
		return false
	}
	if IsRootInventoryGroupFile(p) || ReservedRootInventoryFiles[p] {
		return true
	}
	if strings.HasPrefix(p, "group_vars/") && strings.HasSuffix(p, ".yml") {
		return true
	}
	if strings.HasPrefix(p, "host_vars/") && strings.HasSuffix(p, ".yml") {
		return true
	}
	return false
}

// IsRootInventoryGroupFile reports Ansible group files at inventory root (e.g. lab-ha01.yml).
func IsRootInventoryGroupFile(p string) bool {
	p = NormalizeInventoryPath(p)
	if !strings.HasSuffix(p, ".yml") || strings.Contains(p, "/") {
		return false
	}
	if ReservedRootInventoryFiles[p] || HubCanonicalInventoryFiles[p] || LegacyClusterInventoryPath(p) || JumphostOnlyRootFiles[p] || p == "catalog.yml" {
		return false
	}
	return true
}

// RootInventoryGroupName returns the group name from a root inventory group file path.
func RootInventoryGroupName(p string) string {
	if !IsRootInventoryGroupFile(p) {
		return ""
	}
	return strings.TrimSuffix(NormalizeInventoryPath(p), ".yml")
}

// HubOnlyInventoryPath returns true for paths never overwritten by jumphost import.
func HubOnlyInventoryPath(p string) bool {
	p = NormalizeInventoryPath(p)
	return strings.HasPrefix(p, "_meta/")
}

// LegacyClusterInventoryPath returns true for deprecated cluster paths (migrate on read).
func LegacyClusterInventoryPath(p string) bool {
	p = NormalizeInventoryPath(p)
	if strings.HasPrefix(p, "clusters/") && strings.HasSuffix(p, ".yml") {
		return true
	}
	return p == "clusters.yml"
}

// WritableHubOnlyInventoryPath returns hub-only paths editable via the file API.
func WritableHubOnlyInventoryPath(p string) bool {
	p = NormalizeInventoryPath(p)
	return p == "_meta/status.json"
}

// InventoryPathAllowed returns true when path may be stored via file CRUD.
func InventoryPathAllowed(p string) bool {
	p = NormalizeInventoryPath(p)
	if p == "" || JumphostOnlyRootFiles[p] || strings.HasPrefix(p, "generated/") {
		return false
	}
	if WritableHubOnlyInventoryPath(p) {
		return true
	}
	if HubCanonicalInventoryFiles[p] {
		return true
	}
	if pathSynced(p) {
		return true
	}
	return false
}

// NormalizeInventoryPath cleans a logical inventory path.
func NormalizeInventoryPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	p = path.Clean(p)
	if p == "." || strings.Contains(p, "..") {
		return ""
	}
	return p
}

// FileKey encodes a logical path as a ConfigMap data key (K8s-safe: [-._a-zA-Z0-9]+).
func FileKey(logicalPath string) string {
	logicalPath = NormalizeInventoryPath(logicalPath)
	if logicalPath == "" {
		return ""
	}
	return fileKeyPrefix + strings.ReplaceAll(logicalPath, "/", ".")
}

// PathFromFileKey decodes a ConfigMap data key to a logical path.
func PathFromFileKey(key string) (string, bool) {
	return pathFromLegacyFileKey(key)
}

// ListInventoryFiles returns logical paths from CM data keys.
func ListInventoryFiles(data map[string]string) []InventoryFileInfo {
	if data == nil {
		return nil
	}
	var out []InventoryFileInfo
	for key, content := range data {
		if p, ok := PathFromFileKey(key); ok {
			out = append(out, InventoryFileInfo{
				Path:   p,
				Synced: pathSynced(p),
				Size:   len(content),
			})
		}
	}
	sortFileInfos(out)
	return out
}

func sortFileInfos(in []InventoryFileInfo) {
	for i := 0; i < len(in); i++ {
		for j := i + 1; j < len(in); j++ {
			if in[j].Path < in[i].Path {
				in[i], in[j] = in[j], in[i]
			}
		}
	}
}

// GetInventoryFile returns file content by logical path.
func GetInventoryFile(data map[string]string, logicalPath string) (string, bool) {
	if data == nil {
		return "", false
	}
	key := FileKey(logicalPath)
	if key == "" {
		return "", false
	}
	content, ok := data[key]
	return content, ok
}

// BuildSyncBundle collects synced files for jumphost push.
func BuildSyncBundle(data map[string]string) (ProjectBundle, error) {
	if data == nil {
		return ProjectBundle{}, fmt.Errorf("inventory store is empty")
	}
	files := map[string]string{}
	for key, content := range data {
		p, ok := PathFromFileKey(key)
		if !ok || !pathSynced(p) {
			continue
		}
		files[p] = content
	}
	if _, ok := files["guests.yml"]; !ok {
		return ProjectBundle{}, fmt.Errorf("sync bundle missing guests.yml")
	}
	if _, ok := files["hypervisors.yml"]; !ok {
		return ProjectBundle{}, fmt.Errorf("sync bundle missing hypervisors.yml")
	}
	if _, ok := files["group_vars/all/all.yml"]; !ok {
		return ProjectBundle{}, fmt.Errorf("sync bundle missing group_vars/all/all.yml")
	}
	return ProjectBundle{Files: files}, nil
}

// NormalizeMetaKeys migrates legacy meta/revisions keys and file key encoding in-place on a data map copy.
func NormalizeMetaKeys(data map[string]string) map[string]string {
	if data == nil {
		return map[string]string{}
	}
	out := map[string]string{}
	for k, v := range data {
		out[k] = v
	}
	if _, ok := out[metaKeyMeta]; !ok {
		if v, ok := out[legacyMetaKey]; ok {
			out[metaKeyMeta] = v
			delete(out, legacyMetaKey)
		}
	}
	if _, ok := out[metaKeyRevisions]; !ok {
		if v, ok := out[legacyRevisionsKey]; ok {
			out[metaKeyRevisions] = v
			delete(out, legacyRevisionsKey)
		}
	}
	migrateLegacyFileKeys(out)
	return out
}

// migrateLegacyFileKeys re-encodes invalid or alternate file.* CM keys to FileKey format.
func migrateLegacyFileKeys(data map[string]string) {
	var rekeys []struct{ oldKey, newKey string }
	for k := range data {
		if !strings.HasPrefix(k, fileKeyPrefix) {
			continue
		}
		p, ok := pathFromLegacyFileKey(k)
		if !ok {
			rest := strings.TrimPrefix(k, fileKeyPrefix)
			if strings.Contains(rest, "~") {
				p = strings.ReplaceAll(rest, "~", "/")
			} else {
				continue
			}
		}
		newKey := FileKey(p)
		if newKey != "" && newKey != k {
			rekeys = append(rekeys, struct{ oldKey, newKey string }{k, newKey})
		}
	}
	for _, rk := range rekeys {
		if _, exists := data[rk.newKey]; !exists {
			data[rk.newKey] = data[rk.oldKey]
		}
		delete(data, rk.oldKey)
	}
}

func pathFromLegacyFileKey(key string) (string, bool) {
	if !strings.HasPrefix(key, fileKeyPrefix) {
		return "", false
	}
	rest := strings.TrimPrefix(key, fileKeyPrefix)
	if rest == "" {
		return "", false
	}
	switch {
	case strings.HasPrefix(rest, "host_vars."):
		return decodePrefixedInventoryPath("host_vars/", strings.TrimPrefix(rest, "host_vars.")), true
	case strings.HasPrefix(rest, "group_vars."):
		return decodePrefixedInventoryPath("group_vars/", strings.TrimPrefix(rest, "group_vars.")), true
	case strings.HasPrefix(rest, "clusters."):
		return decodePrefixedInventoryPath("clusters/", strings.TrimPrefix(rest, "clusters.")), true
	default:
		return rest, true
	}
}

func decodePrefixedInventoryPath(prefix, encoded string) string {
	parts := strings.Split(encoded, ".")
	if len(parts) >= 2 && parts[len(parts)-1] == "yml" {
		return prefix + strings.Join(parts[:len(parts)-1], "/") + ".yml"
	}
	return prefix + strings.ReplaceAll(encoded, ".", "/")
}

func metaRaw(data map[string]string) string {
	data = NormalizeMetaKeys(data)
	if v := data[metaKeyMeta]; v != "" {
		return v
	}
	return data[legacyMetaKey]
}

func revisionsRaw(data map[string]string) string {
	data = NormalizeMetaKeys(data)
	if v := data[metaKeyRevisions]; v != "" {
		return v
	}
	return data[legacyRevisionsKey]
}

func statusRaw(data map[string]string) string {
	data = NormalizeMetaKeys(data)
	return data[metaKeyStatus]
}

// StatusContent returns _meta/status.json body or {}.
func StatusContent(data map[string]string) string {
	if v := statusRaw(data); strings.TrimSpace(v) != "" {
		return v
	}
	return "{}"
}

// InventoryStoreSize returns total byte size of CM data.
func InventoryStoreSize(data map[string]string) int {
	n := 0
	for _, v := range data {
		n += len(v)
	}
	return n
}

// MetaKeyRevisions returns the CM key for revision log.
func MetaKeyRevisions() string { return metaKeyRevisions }

// MetaKeyImportPreview returns the CM key for import preview cache.
func MetaKeyImportPreview() string { return metaKeyImportPreview }

const maxInventoryStoreBytes = 900 * 1024

// CheckInventoryStoreSize warns when approaching ConfigMap limit.
func CheckInventoryStoreSize(data map[string]string) error {
	if InventoryStoreSize(data) > maxInventoryStoreBytes {
		return fmt.Errorf("inventory store exceeds %d bytes (ConfigMap limit ~1MiB)", maxInventoryStoreBytes)
	}
	return nil
}
