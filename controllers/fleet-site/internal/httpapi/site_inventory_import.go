package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
)

const importPreviewMetaKey = "_meta.import_preview.json"

func (s *Server) previewSiteInventoryImport(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName string, fs *fleetv1alpha1.FleetSite) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	if provisionActionBusy(fs) {
		http.Error(w, "site provision action already running", http.StatusConflict)
		return
	}
	jobName, err := s.startInventoryImportPreviewJob(ctx, fs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"jobName": jobName, "phase": "running"})
}

func (s *Server) storeImportPreview(ctx context.Context, siteName string, changes []yaidap.ImportFileChange) error {
	raw, err := json.Marshal(map[string]interface{}{"changes": changes})
	if err != nil {
		return err
	}
	return connectors.PatchInventory(ctx, s.Client, s.Cfg.Namespace, siteName, fleetv1alpha1.ProvisionProviderYaidap, func(data map[string]string) (connectors.InventoryPatch, error) {
		return connectors.InventoryPatch{
			FilePut: map[string]string{"_meta/import_preview.json": string(raw)},
		}, nil
	})
}

func (s *Server) collectImportPreview(ctx context.Context, fs *fleetv1alpha1.FleetSite) ([]yaidap.ImportFileChange, error) {
	s.refreshInventoryJobs(ctx, fs)
	data, err := connectors.Load(ctx, s.Client, s.Cfg.Namespace, fs.Name, fleetv1alpha1.ProvisionProviderYaidap)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	data = connectors.NormalizeMetaKeys(data)
	raw := data[connectors.MetaKeyImportPreview()]
	if raw == "" {
		return nil, nil
	}
	var stored struct {
		Changes []yaidap.ImportFileChange `json:"changes"`
	}
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, err
	}
	return stored.Changes, nil
}

func (s *Server) getSiteInventoryImportPreview(ctx context.Context, w http.ResponseWriter, siteName string) {
	fs, ok := s.loadFleetSite(ctx, w, siteName)
	if !ok {
		return
	}
	s.refreshInventoryJobs(ctx, fs)
	changes, err := s.collectImportPreview(ctx, fs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": changes})
}
