package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
)

func (s *Server) handleSiteInventoryFileRoutes(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, rest string) bool {
	switch {
	case rest == "provision/inventory/files":
		switch r.Method {
		case http.MethodGet:
			s.listSiteInventoryFiles(ctx, w, siteName)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return true
	case strings.HasPrefix(rest, "provision/inventory/files/"):
		path := strings.TrimPrefix(rest, "provision/inventory/files/")
		path = strings.Trim(path, "/")
		if path == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return true
		}
		switch r.Method {
		case http.MethodGet:
			s.getSiteInventoryFile(ctx, w, siteName, path)
		case http.MethodPut:
			s.putSiteInventoryFile(ctx, w, r, siteName, path)
		case http.MethodDelete:
			s.deleteSiteInventoryFile(ctx, w, r, siteName, path)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return true
	case rest == "provision/inventory/hosts":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		s.listSiteInventoryHosts(ctx, w, siteName)
		return true
	case rest == "provision/inventory/clusters":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		s.listSiteInventoryClusters(ctx, w, siteName)
		return true
	case rest == "provision/inventory/migrate":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		s.migrateSiteInventory(ctx, w, r, siteName)
		return true
	case rest == "provision/inventory/import/preview":
		switch r.Method {
		case http.MethodPost:
			fs, ok := s.loadFleetSite(ctx, w, siteName)
			if !ok {
				return true
			}
			s.previewSiteInventoryImport(ctx, w, r, siteName, fs)
		case http.MethodGet:
			s.getSiteInventoryImportPreview(ctx, w, siteName)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return true
	case rest == "provision/inventory/import/apply":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		fs, ok := s.loadFleetSite(ctx, w, siteName)
		if !ok {
			return true
		}
		s.applySiteInventoryImport(ctx, w, r, siteName, fs)
		return true
	}
	return false
}

func (s *Server) listSiteInventoryFiles(ctx context.Context, w http.ResponseWriter, siteName string) {
	_ = yaidap.MigrateLegacyStore(ctx, s.Client, s.Cfg.Namespace, siteName, "system/migrate")
	files, err := yaidap.ListInventoryFiles(ctx, s.Client, s.Cfg.Namespace, siteName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": files})
}

func (s *Server) getSiteInventoryFile(ctx context.Context, w http.ResponseWriter, siteName, path string) {
	content, ok, err := yaidap.GetInventoryFileContent(ctx, s.Client, s.Cfg.Namespace, siteName, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(content))
}

func (s *Server) putSiteInventoryFile(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, path string) {
	actor, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := yaidap.PutInventoryFile(ctx, s.Client, s.Cfg.Namespace, siteName, path, string(body), connectors.FormatActor(actor.Kind, actor.Name)); err != nil {
		if err == connectors.ErrInvalidInventoryPath {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path, "syncPhase": "dirty"})
}

func (s *Server) deleteSiteInventoryFile(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, path string) {
	actor, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	if err := yaidap.DeleteInventoryFile(ctx, s.Client, s.Cfg.Namespace, siteName, path, connectors.FormatActor(actor.Kind, actor.Name)); err != nil {
		if err == connectors.ErrInventoryFileNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listSiteInventoryHosts(ctx context.Context, w http.ResponseWriter, siteName string) {
	_ = yaidap.MigrateLegacyStore(ctx, s.Client, s.Cfg.Namespace, siteName, "system/migrate")
	hosts, err := yaidap.ListInventoryHosts(ctx, s.Client, s.Cfg.Namespace, siteName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": hosts})
}

func (s *Server) listSiteInventoryClusters(ctx context.Context, w http.ResponseWriter, siteName string) {
	doc, err := yaidap.LoadClustersFile(ctx, s.Client, s.Cfg.Namespace, siteName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) migrateSiteInventory(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName string) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	if err := yaidap.MigrateLegacyStore(ctx, s.Client, s.Cfg.Namespace, siteName, "system/migrate"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "migrated"})
}

type importApplyBody struct {
	Paths   []string `json:"paths"`
	Confirm bool     `json:"confirm"`
}

func (s *Server) applySiteInventoryImport(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName string, fs *fleetv1alpha1.FleetSite) {
	actor, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req importApplyBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if !req.Confirm {
		http.Error(w, "confirm=true required", http.StatusBadRequest)
		return
	}
	preview, err := s.collectImportPreview(ctx, fs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	selected := map[string]bool{}
	for _, p := range req.Paths {
		selected[connectors.NormalizeInventoryPath(p)] = true
	}
	put := map[string]string{}
	for _, ch := range preview {
		if len(selected) > 0 && !selected[ch.Path] {
			continue
		}
		if ch.Status == "unchanged" || ch.Remote == "" {
			continue
		}
		put[ch.Path] = ch.Remote
	}
	if len(put) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"status": "no changes"})
		return
	}
	put, err = yaidap.NormalizeImportPut(put)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	warnings := importApplyWarnings(put)
	if err := connectors.PutInventoryFiles(ctx, s.Client, s.Cfg.Namespace, siteName, fleetv1alpha1.ProvisionProviderYaidap, connectors.FormatActor(actor.Kind, actor.Name), put, yaidap.ValidateInventoryFile); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := map[string]interface{}{"applied": len(put), "paths": req.Paths}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	writeJSON(w, http.StatusOK, resp)
}

func importApplyWarnings(put map[string]string) []string {
	var out []string
	for path, content := range put {
		out = append(out, yaidap.InventoryPlaceholderWarnings(path, content)...)
	}
	return out
}
