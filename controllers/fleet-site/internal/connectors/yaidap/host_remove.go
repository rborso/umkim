package yaidap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RemoveHostDeclaration drops a host from the site inventory: its entry in
// guests.yml / bare_metals.yml, its host_vars / group_vars files, and its status
// record. The VM must already be gone (destroy); this only removes the
// bookkeeping. Shared by the host-remove API and by cluster delete when it is
// asked to clean up VM declarations after destroying the guests.
func RemoveHostDeclaration(ctx context.Context, c client.Client, ns, siteName, hostName string) error {
	return connectors.PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
		patch := connectors.InventoryPatch{}
		// Host entry: guests.yml for VMs, bare_metals.yml for bare metal.
		for _, file := range []string{"guests.yml", "bare_metals.yml"} {
			content := storeFileContent(data, file)
			if content == "" {
				continue
			}
			if updated, ok := removeHostBlock(content, hostName); ok {
				patch.FilePut = map[string]string{file: updated}
			}
		}
		// Per-host vars.
		deletes := []string{}
		for _, file := range []string{"host_vars/" + hostName + ".yml", "group_vars/" + hostName + ".yml"} {
			if storeFileContent(data, file) != "" {
				deletes = append(deletes, file)
			}
		}
		patch.FileDelete = deletes
		// Status record.
		doc := loadStatusDocFor(data)
		if _, ok := doc.Guests[hostName]; ok {
			delete(doc.Guests, hostName)
			raw, err := json.Marshal(doc)
			if err != nil {
				return connectors.InventoryPatch{}, err
			}
			if patch.FilePut == nil {
				patch.FilePut = map[string]string{}
			}
			patch.FilePut["_meta/status.json"] = string(raw)
		}
		return patch, nil
	})
}

// removeHostBlock drops a top-level host entry (and its indented fields) from a
// guests/bare_metals document, preserving the rest of the file verbatim. The
// entry indent is taken from the surrounding `hosts:` key rather than assumed,
// so both the 2-space templates and the 4-space files written by earlier
// yaml.Marshal calls are handled; only lines indented deeper than the entry
// itself are consumed, so the following hosts survive.
func removeHostBlock(content, host string) (string, bool) {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	removed := false
	hostsIndent := -1
	skipIndent := -1
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if skipIndent >= 0 {
			if trimmed == "" {
				continue
			}
			if indent > skipIndent {
				continue
			}
			skipIndent = -1
		}
		if trimmed == "hosts:" {
			hostsIndent = indent
			out = append(out, line)
			continue
		}
		if hostsIndent >= 0 && indent > hostsIndent && trimmed == host+":" {
			skipIndent = indent
			removed = true
			continue
		}
		out = append(out, line)
	}
	if !removed {
		return content, false
	}
	return strings.Join(out, "\n"), true
}

// storeFileContent reads a logical inventory file out of a store snapshot. The
// store keys files as "file.<path with dots>".
func storeFileContent(data map[string]string, path string) string {
	if data == nil {
		return ""
	}
	for _, k := range []string{"file." + strings.ReplaceAll(path, "/", "."), "file." + path, path} {
		if v, ok := data[k]; ok {
			return v
		}
	}
	return ""
}

// RemoveClusterGroupMembers drops hosts from a cluster root group file
// (<cluster>.yml) and deletes that file once no members remain. Cluster delete
// removes the guests it declared; the group file that tied them to the cluster
// would otherwise be left behind.
func RemoveClusterGroupMembers(ctx context.Context, c client.Client, ns, siteName, cluster string, hosts []string) error {
	cluster = strings.TrimSpace(cluster)
	if cluster == "" || len(hosts) == 0 {
		return nil
	}
	path := cluster + ".yml"
	return connectors.PatchInventory(ctx, c, ns, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
		content := storeFileContent(data, path)
		if strings.TrimSpace(content) == "" {
			return connectors.InventoryPatch{}, nil
		}
		updated, dropFile, changed, err := pruneClusterGroup(content, cluster, hosts)
		if err != nil {
			return connectors.InventoryPatch{}, fmt.Errorf("%s: %w", path, err)
		}
		if !changed {
			return connectors.InventoryPatch{}, nil
		}
		if dropFile {
			return connectors.InventoryPatch{FileDelete: []string{path}}, nil
		}
		return connectors.InventoryPatch{FilePut: map[string]string{path: updated}}, nil
	})
}

// pruneClusterGroup removes hosts from a cluster root group file. It returns the
// updated content, whether the file should be deleted (no members left), and
// whether anything changed.
func pruneClusterGroup(content, cluster string, hosts []string) (updated string, dropFile, changed bool, err error) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return "", false, false, err
	}
	group, _ := doc[cluster].(map[string]interface{})
	if group == nil {
		return "", false, false, nil
	}
	members, _ := group["hosts"].(map[string]interface{})
	if members == nil {
		return "", false, false, nil
	}
	removed := false
	for _, h := range hosts {
		if _, ok := members[h]; ok {
			delete(members, h)
			removed = true
		}
	}
	if !removed {
		return "", false, false, nil
	}
	if len(members) == 0 {
		delete(doc, cluster)
	}
	if len(doc) == 0 {
		return "", true, true, nil
	}
	rendered, err := marshalInventoryYAML(doc)
	if err != nil {
		return "", false, false, err
	}
	return rendered, false, true, nil
}

// loadStatusDocFor parses _meta/status.json out of a store snapshot.
func loadStatusDocFor(data map[string]string) InventoryStatusDoc {
	raw := connectors.StatusContent(data)
	doc := InventoryStatusDoc{}
	_ = json.Unmarshal([]byte(raw), &doc)
	if doc.Guests == nil {
		doc.Guests = map[string]GuestStatus{}
	}
	return doc
}
