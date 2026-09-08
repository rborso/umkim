package yaidap

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
)

// ImportFileChange describes one file difference between hub and jumphost.
type ImportFileChange struct {
	Path   string `json:"path"`
	Status string `json:"status"` // unchanged, added, modified, deleted
	Diff   string `json:"diff,omitempty"`
	Remote string `json:"remote,omitempty"`
	Local  string `json:"local,omitempty"`
}

// CompareImportFiles diffs remote (jumphost) files against hub store.
func CompareImportFiles(localData map[string]string, remoteFiles map[string]string) []ImportFileChange {
	localData = connectors.NormalizeMetaKeys(localData)
	var paths []string
	seen := map[string]bool{}
	for p := range remoteFiles {
		if connectors.HubOnlyInventoryPath(p) {
			continue
		}
		if !connectors.InventoryPathAllowed(p) || !pathSyncedForImport(p) {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	for _, info := range connectors.ListInventoryFiles(localData) {
		if !info.Synced {
			continue
		}
		if !seen[info.Path] {
			paths = append(paths, info.Path)
		}
	}
	sortPaths(paths)
	var out []ImportFileChange
	for _, p := range paths {
		local, lok := connectors.GetInventoryFile(localData, p)
		remote, rok := remoteFiles[p]
		switch {
		case lok && rok && strings.TrimSpace(local) == strings.TrimSpace(remote):
			out = append(out, ImportFileChange{Path: p, Status: "unchanged"})
		case !lok && rok:
			out = append(out, ImportFileChange{
				Path:   p,
				Status: "added",
				Diff:   unifiedDiff("", remote, p),
				Remote: remote,
			})
		case lok && !rok:
			out = append(out, ImportFileChange{
				Path:  p,
				Status: "deleted",
				Diff:  unifiedDiff(local, "", p),
				Local: local,
			})
		default:
			out = append(out, ImportFileChange{
				Path:   p,
				Status: "modified",
				Diff:   unifiedDiff(local, remote, p),
				Local:  local,
				Remote: remote,
			})
		}
	}
	return out
}

func pathSyncedForImport(p string) bool {
	return connectors.InventoryPathAllowed(p) && !connectors.HubOnlyInventoryPath(p)
}

// ApplyImportFiles merges selected remote files into hub store.
func ApplyImportFiles(localData map[string]string, remoteFiles map[string]string, paths []string) map[string]string {
	put := map[string]string{}
	for _, p := range paths {
		p = connectors.NormalizeInventoryPath(p)
		if p == "" || connectors.HubOnlyInventoryPath(p) {
			continue
		}
		if content, ok := remoteFiles[p]; ok {
			put[p] = content
		}
	}
	return put
}

func unifiedDiff(before, after, path string) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(before, after, true)
	diffs = dmp.DiffCleanupSemantic(diffs)
	patch := dmp.PatchMake(before, diffs)
	text := dmp.PatchToText(patch)
	if strings.TrimSpace(text) == "" {
		return fmt.Sprintf("--- %s\n+++ %s\n(content changed)", path, path)
	}
	return text
}

func sortPaths(paths []string) {
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			if paths[j] < paths[i] {
				paths[i], paths[j] = paths[j], paths[i]
			}
		}
	}
}

// ParseImportFilesJSON decodes import preview payload from connector job.
func ParseImportFilesJSON(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty import files payload")
	}
	var out map[string]string
	if err := jsonDecode([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("invalid import files JSON: %w", err)
	}
	return out, nil
}
