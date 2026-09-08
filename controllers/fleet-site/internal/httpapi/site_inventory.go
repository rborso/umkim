package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/tofu"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/provision"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitehosts"
	"k8s.io/apimachinery/pkg/types"
)

func (s *Server) handleSiteInventoryRoutes(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, rest string) {
	if s.handleSiteInventoryFileRoutes(ctx, w, r, siteName, rest) {
		return
	}
	fs, ok := s.loadFleetSite(ctx, w, siteName)
	if !ok {
		return
	}
	provider := fs.Spec.EffectiveProvisionProvider()
	switch {
	case rest == "provision/inventory":
		switch r.Method {
		case http.MethodGet:
			s.getSiteInventory(ctx, w, siteName, provider)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case rest == "provision/inventory/catalog":
		switch r.Method {
		case http.MethodGet:
			s.getSiteInventoryCatalog(ctx, w, siteName, provider)
		case http.MethodPut, http.MethodPatch:
			s.patchSiteInventoryCatalog(ctx, w, r, siteName, provider)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case rest == "provision/inventory/guests":
		switch r.Method {
		case http.MethodGet:
			s.listSiteInventoryGuests(ctx, w, siteName, provider)
		case http.MethodPost:
			s.createSiteInventoryGuest(ctx, w, r, siteName, provider)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case strings.HasPrefix(rest, "provision/inventory/guests/"):
		sub := strings.TrimPrefix(rest, "provision/inventory/guests/")
		sub = strings.Trim(sub, "/")
		parts := strings.SplitN(sub, "/", 2)
		guestName := parts[0]
		if guestName == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.getSiteInventoryGuest(ctx, w, siteName, provider, guestName)
		case http.MethodPut, http.MethodPatch:
			s.patchSiteInventoryGuest(ctx, w, r, siteName, provider, guestName)
		case http.MethodDelete:
			s.deleteSiteInventoryGuest(ctx, w, r, siteName, provider, guestName)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case rest == "provision/inventory/revisions":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.listSiteInventoryRevisions(ctx, w, r, siteName, provider)
	case strings.HasPrefix(rest, "provision/inventory/revisions/"):
		id := strings.TrimPrefix(rest, "provision/inventory/revisions/")
		id = strings.Trim(id, "/")
		if id == "" || r.Method != http.MethodGet {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		s.getSiteInventoryRevision(ctx, w, siteName, provider, id)
	case rest == "provision/inventory/sync":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.syncSiteInventory(ctx, w, r, siteName, fs)
	case rest == "provision/inventory/import":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.importSiteInventory(ctx, w, r, siteName, fs)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) loadFleetSite(ctx context.Context, w http.ResponseWriter, siteName string) (*fleetv1alpha1.FleetSite, bool) {
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return nil, false
	}
	return &fs, true
}

func (s *Server) rejectWrongProvider(w http.ResponseWriter, provider, want string) bool {
	if provider == want || (want == fleetv1alpha1.ProvisionProviderYaidap && provider == fleetv1alpha1.ProvisionProviderYaidap) {
		return false
	}
	if provider == fleetv1alpha1.ProvisionProviderNoop {
		http.Error(w, "site provision provider is noop", http.StatusConflict)
		return true
	}
	http.Error(w, "inventory API not available for provider "+provider, http.StatusConflict)
	return true
}

func (s *Server) getSiteInventory(ctx context.Context, w http.ResponseWriter, siteName, provider string) {
	if s.rejectWrongProvider(w, provider, fleetv1alpha1.ProvisionProviderYaidap) {
		return
	}
	fs, ok := s.loadFleetSite(ctx, w, siteName)
	if ok && fs != nil {
		s.refreshInventoryJobs(ctx, fs)
		if provider == fleetv1alpha1.ProvisionProviderYaidap {
			_ = yaidap.MigrateLegacyStore(ctx, s.Client, s.Cfg.Namespace, siteName, "system/migrate")
		}
	}
	meta, err := connectors.LoadMeta(ctx, s.Client, s.Cfg.Namespace, siteName, provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	files, _ := yaidap.ListInventoryFiles(ctx, s.Client, s.Cfg.Namespace, siteName)
	env := connectors.InventoryEnvelope{Meta: meta, Files: files}
	if provider == fleetv1alpha1.ProvisionProviderYaidap {
		catalog, guests, _, err := yaidap.LoadBundle(ctx, s.Client, s.Cfg.Namespace, siteName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		env.Catalog = catalog
		env.Guests = guests
	} else if provider == fleetv1alpha1.ProvisionProviderOpenTofu || provider == fleetv1alpha1.ProvisionProviderTerraform {
		catalog, guests, _, err := tofu.LoadBundle(ctx, s.Client, s.Cfg.Namespace, siteName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		env.Catalog = catalog
		env.Guests = guests
	}
	revisions, err := connectors.ListRevisions(ctx, s.Client, s.Cfg.Namespace, siteName, provider, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	env.Revisions = revisions
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) getSiteInventoryCatalog(ctx context.Context, w http.ResponseWriter, siteName, provider string) {
	if s.rejectWrongProvider(w, provider, provider) {
		return
	}
	switch provider {
	case fleetv1alpha1.ProvisionProviderYaidap:
		catalog, _, _, err := yaidap.LoadBundle(ctx, s.Client, s.Cfg.Namespace, siteName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, catalog)
	case fleetv1alpha1.ProvisionProviderOpenTofu, fleetv1alpha1.ProvisionProviderTerraform:
		catalog, _, _, err := tofu.LoadBundle(ctx, s.Client, s.Cfg.Namespace, siteName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, catalog)
	default:
		http.Error(w, "inventory API not available for provider "+provider, http.StatusConflict)
	}
}

func (s *Server) patchSiteInventoryCatalog(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, provider string) {
	if provider == fleetv1alpha1.ProvisionProviderYaidap {
		http.Error(w, inventoryJSONDeprecated, http.StatusGone)
		return
	}
	if s.rejectWrongProvider(w, provider, provider) {
		return
	}
	http.Error(w, "tofu catalog write not implemented yet", http.StatusNotImplemented)
}

func mergeYaidapCatalog(dst, patch yaidap.Catalog) yaidap.Catalog {
	if len(patch.Networks) > 0 {
		dst.Networks = patch.Networks
	}
	if len(patch.Storages) > 0 {
		dst.Storages = patch.Storages
	}
	if len(patch.Hypervisors) > 0 {
		dst.Hypervisors = patch.Hypervisors
	}
	if patch.Defaults != (yaidap.GuestDefaults{}) {
		dst.Defaults = patch.Defaults
	}
	return dst
}

func (s *Server) listSiteInventoryGuests(ctx context.Context, w http.ResponseWriter, siteName, provider string) {
	store, err := s.loadGuestStore(ctx, siteName, provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": guestList(store)})
}

func guestList(store interface{}) []interface{} {
	switch s := store.(type) {
	case yaidap.GuestStore:
		out := make([]interface{}, 0, len(s))
		for name, spec := range s {
			out = append(out, map[string]interface{}{"name": name, "spec": spec})
		}
		return out
	case tofu.GuestStore:
		out := make([]interface{}, 0, len(s))
		for name, spec := range s {
			out = append(out, map[string]interface{}{"name": name, "spec": spec})
		}
		return out
	default:
		return nil
	}
}

func (s *Server) loadGuestStore(ctx context.Context, siteName, provider string) (interface{}, error) {
	switch provider {
	case fleetv1alpha1.ProvisionProviderYaidap:
		_, guests, _, err := yaidap.LoadBundle(ctx, s.Client, s.Cfg.Namespace, siteName)
		return guests, err
	case fleetv1alpha1.ProvisionProviderOpenTofu, fleetv1alpha1.ProvisionProviderTerraform:
		_, guests, _, err := tofu.LoadBundle(ctx, s.Client, s.Cfg.Namespace, siteName)
		return guests, err
	default:
		return nil, nil
	}
}

const inventoryJSONDeprecated = "JSON guest/catalog mutations are deprecated; edit YAML via /provision/inventory/files"

func (s *Server) createSiteInventoryGuest(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, provider string) {
	http.Error(w, inventoryJSONDeprecated, http.StatusGone)
}

func (s *Server) getSiteInventoryGuest(ctx context.Context, w http.ResponseWriter, siteName, provider, guestName string) {
	switch provider {
	case fleetv1alpha1.ProvisionProviderYaidap:
		_, guests, _, err := yaidap.LoadBundle(ctx, s.Client, s.Cfg.Namespace, siteName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		spec, ok := guests[guestName]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"name": guestName, "spec": spec})
	case fleetv1alpha1.ProvisionProviderOpenTofu, fleetv1alpha1.ProvisionProviderTerraform:
		_, guests, _, err := tofu.LoadBundle(ctx, s.Client, s.Cfg.Namespace, siteName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		spec, ok := guests[guestName]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"name": guestName, "spec": spec})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) patchSiteInventoryGuest(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, provider, guestName string) {
	http.Error(w, inventoryJSONDeprecated, http.StatusGone)
}

func (s *Server) deleteSiteInventoryGuest(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, provider, guestName string) {
	http.Error(w, inventoryJSONDeprecated, http.StatusGone)
}

func (s *Server) listSiteInventoryRevisions(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, provider string) {
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := connectors.ListRevisions(ctx, s.Client, s.Cfg.Namespace, siteName, provider, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (s *Server) getSiteInventoryRevision(ctx context.Context, w http.ResponseWriter, siteName, provider, id string) {
	rev, ok, err := connectors.GetRevision(ctx, s.Client, s.Cfg.Namespace, siteName, provider, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, rev)
}

func (s *Server) syncSiteInventory(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName string, fs *fleetv1alpha1.FleetSite) {
	actor, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	if provisionActionBusy(fs) {
		http.Error(w, "site provision action already running", http.StatusConflict)
		return
	}
	provider := fs.Spec.EffectiveProvisionProvider()
	switch provider {
	case fleetv1alpha1.ProvisionProviderYaidap:
		if err := provision.ValidateYaidapBinding(fs.Spec.Provision); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := yaidap.EnsureReadyForProvision(ctx, s.Client, s.Cfg.Namespace, siteName, connectors.FormatActor(actor.Kind, actor.Name)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := yaidap.MarkSyncing(ctx, s.Client, s.Cfg.Namespace, siteName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case fleetv1alpha1.ProvisionProviderOpenTofu, fleetv1alpha1.ProvisionProviderTerraform:
		http.Error(w, "tofu inventory sync not implemented yet", http.StatusNotImplemented)
		return
	default:
		http.Error(w, "inventory sync not available for provider "+provider, http.StatusConflict)
		return
	}
	jobName, err := s.startInventorySyncJob(ctx, fs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"jobName": jobName, "phase": "running"})
}

func (s *Server) importSiteInventory(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName string, fs *fleetv1alpha1.FleetSite) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	if provisionActionBusy(fs) {
		http.Error(w, "site provision action already running", http.StatusConflict)
		return
	}
	provider := fs.Spec.EffectiveProvisionProvider()
	if provider != fleetv1alpha1.ProvisionProviderYaidap {
		if provider == fleetv1alpha1.ProvisionProviderNoop {
			http.Error(w, "noop provider has no inventory import", http.StatusConflict)
			return
		}
		http.Error(w, "inventory import supported for yaidap only", http.StatusConflict)
		return
	}
	if err := provision.ValidateYaidapBinding(fs.Spec.Provision); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	jobName, err := s.startInventoryImportJob(ctx, fs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"jobName": jobName, "phase": "running"})
}

// mergeImportHandoff updates hub site hosts from imported guest specs without clobbering clusterIntent.
func (s *Server) mergeImportHandoff(ctx context.Context, siteName string, guests yaidap.GuestStore) error {
	for name, spec := range guests {
		if spec.Absent {
			continue
		}
		rec, found, err := sitehosts.Get(ctx, s.Client, s.Cfg.Namespace, siteName, name)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		patch := rec
		if ip := strings.TrimSpace(spec.AnsibleHost); ip != "" {
			patch.AnsibleHost = ip
		}
		if loc := strings.TrimSpace(spec.VmLocation); loc != "" {
			patch.VmLocation = loc
		}
		if err := sitehosts.Put(ctx, s.Client, s.Cfg.Namespace, siteName, patch); err != nil {
			return err
		}
	}
	return nil
}
