package httpapi

import (
	"context"
	"io"
	"net/http"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitecluster"
)

func (s *Server) handleSiteHostCluster(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, hostName string) {
	switch r.Method {
	case http.MethodGet:
		s.getSiteHostCluster(ctx, w, siteName, hostName)
	case http.MethodPost:
		s.applySiteHostCluster(ctx, w, r, siteName, hostName)
	case http.MethodPatch, http.MethodPut:
		s.patchSiteHostCluster(ctx, w, r, siteName, hostName)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getSiteHostCluster(ctx context.Context, w http.ResponseWriter, siteName, hostName string) {
	views, err := yaidap.ListInventoryHosts(ctx, s.Client, s.Cfg.Namespace, siteName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, v := range views {
		if v.Name == hostName {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"clusters": v.Clusters,
				"cluster":  v.Status.Cluster,
			})
			return
		}
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func (s *Server) patchSiteHostCluster(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, hostName string) {
	_, _ = io.ReadAll(r.Body)
	http.Error(w, "cluster intent is edited in root {cluster}.yml and group_vars/{cluster}.yml", http.StatusGone)
}

func (s *Server) applySiteHostCluster(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, hostName string) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	if err := sitecluster.ApplyInventoryGuestCluster(ctx, s.Client, s.Cfg, siteName, hostName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	views, err := yaidap.ListInventoryHosts(ctx, s.Client, s.Cfg.Namespace, siteName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, v := range views {
		if v.Name == hostName {
			writeJSON(w, http.StatusAccepted, v)
			return
		}
	}
	http.Error(w, "not found", http.StatusNotFound)
}
