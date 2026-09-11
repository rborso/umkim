package httpapi

import (
	"context"
	"net/http"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/yaidapsync"
)

func (s *Server) syncSiteYaidapRepo(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName string, fs *fleetv1alpha1.FleetSite) {
	_, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	if provisionActionBusy(fs) {
		http.Error(w, "site provision action already running", http.StatusConflict)
		return
	}
	if fs.Spec.EffectiveProvisionProvider() != fleetv1alpha1.ProvisionProviderYaidap {
		http.Error(w, "yaidap repo sync requires provider yaidap", http.StatusConflict)
		return
	}
	if err := yaidap.ValidateYaidapRepoSync(fs.Spec.Provision); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	jobName, err := yaidapsync.StartRepoSync(ctx, s.Client, s.Cfg, fs)
	if err != nil {
		if err.Error() == "yaidap repo sync already running" {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"jobName": jobName,
		"phase":   "running",
		"ref":     yaidap.EffectiveYaidapRef(fs.Spec.Provision),
	})
}
