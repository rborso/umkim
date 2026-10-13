package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// removeSiteHost drops a host from the site inventory: its entry in guests.yml /
// bare_metals.yml, its host_vars / group_vars files, and its status record. The
// VM must already be gone (destroy); this only removes the bookkeeping.
func (s *Server) removeSiteHost(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, hostName string) {
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "site not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	host, found, err := yaidap.LookupInventoryHost(ctx, s.Client, s.Cfg.Namespace, siteName, hostName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "host not found", http.StatusNotFound)
		return
	}
	force := strings.EqualFold(r.URL.Query().Get("force"), "true")
	switch host.Status.Phase {
	case fleetv1alpha1.SiteHostPhaseReady, fleetv1alpha1.SiteHostPhaseProvisioning, fleetv1alpha1.SiteHostPhaseDestroying:
		if !force {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "host is " + host.Status.Phase + "; destroy its VM first or pass force=true",
			})
			return
		}
	}

	provider := fs.Spec.EffectiveProvisionProvider()
	err = connectors.PatchInventory(ctx, s.Client, s.Cfg.Namespace, siteName, provider, func(data map[string]string) (connectors.InventoryPatch, error) {
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
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": hostName, "removed": true})
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

// loadStatusDocFor parses _meta/status.json out of a store snapshot.
func loadStatusDocFor(data map[string]string) yaidap.InventoryStatusDoc {
	raw := connectors.StatusContent(data)
	doc := yaidap.InventoryStatusDoc{}
	_ = json.Unmarshal([]byte(raw), &doc)
	if doc.Guests == nil {
		doc.Guests = map[string]yaidap.GuestStatus{}
	}
	return doc
}
