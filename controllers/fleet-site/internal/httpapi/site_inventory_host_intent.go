package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
)

// createSiteInventoryHost implements the guided "add host" intent API: it composes
// the YAIDAP inventory files server-side from a minimal, kind-aware request and
// writes them. It never triggers destructive provision actions.
func (s *Server) createSiteInventoryHost(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName string) {
	actor, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	fs, ok := s.loadFleetSite(ctx, w, siteName)
	if !ok {
		return
	}
	if fs.Spec.EffectiveProvisionProvider() != fleetv1alpha1.ProvisionProviderYaidap {
		http.Error(w, "host intent requires yaidap provider", http.StatusConflict)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req yaidap.HostIntent
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Normalize()
	if err := req.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	catalog, _, _, err := yaidap.LoadBundle(ctx, s.Client, s.Cfg.Namespace, siteName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	current := map[string]string{}
	readPaths := []string{"guests.yml", "bare_metals.yml", "hypervisors.yml"}
	if req.Cluster != "" {
		readPaths = append(readPaths, req.Cluster+".yml")
	}
	// The target hypervisor's own host_vars, so the guest can be attached to
	// the uplink that offers its network rather than to the first one.
	if req.Kind == "vm" && req.Hypervisor != "" {
		readPaths = append(readPaths, "host_vars/"+req.Hypervisor+".yml")
	}
	for _, p := range readPaths {
		content, found, err := yaidap.GetInventoryFileContent(ctx, s.Client, s.Cfg.Namespace, siteName, p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if found {
			current[p] = content
		}
	}

	files, err := yaidap.BuildHostFiles(current, req, catalog)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	warnings := []string{}
	paths := make([]string, 0, len(files))
	for path, content := range files {
		paths = append(paths, path)
		warnings = append(warnings, yaidap.InventoryPlaceholderWarnings(path, content)...)
	}
	sort.Strings(paths)

	if err := connectors.PutInventoryFiles(ctx, s.Client, s.Cfg.Namespace, siteName,
		fleetv1alpha1.ProvisionProviderYaidap, connectors.FormatActor(actor.Kind, actor.Name),
		files, yaidap.ValidateInventoryFile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp := map[string]interface{}{
		"host":  map[string]string{"name": req.Name, "kind": req.Kind},
		"paths": paths,
	}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	writeJSON(w, http.StatusOK, resp)
}
