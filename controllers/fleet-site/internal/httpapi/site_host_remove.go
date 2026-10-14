package httpapi

import (
	"context"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
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

	if err := yaidap.RemoveHostDeclaration(ctx, s.Client, s.Cfg.Namespace, siteName, hostName); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": hostName, "removed": true})
}
