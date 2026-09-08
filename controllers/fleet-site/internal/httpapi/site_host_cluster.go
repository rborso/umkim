package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitecluster"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitehosts"
)

type siteHostClusterBody struct {
	ClusterIntent *fleetv1alpha1.SiteHostClusterIntent `json:"clusterIntent,omitempty"`
}

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
	rec, ok, err := sitehosts.Get(ctx, s.Client, s.Cfg.Namespace, siteName, hostName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"clusterIntent": rec.ClusterIntent,
		"cluster":       rec.Status.Cluster,
	})
}

func (s *Server) patchSiteHostCluster(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, hostName string) {
	_, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req siteHostClusterBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ClusterIntent == nil {
		http.Error(w, "clusterIntent required", http.StatusBadRequest)
		return
	}
	existing, found, err := sitehosts.Get(ctx, s.Client, s.Cfg.Namespace, siteName, hostName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := sitehosts.ValidateClusterIntent(req.ClusterIntent); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	existing.ClusterIntent = req.ClusterIntent
	if err := sitehosts.Put(ctx, s.Client, s.Cfg.Namespace, siteName, existing); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) applySiteHostCluster(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, hostName string) {
	_, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	if err := sitecluster.ApplyHostNow(ctx, s.Client, s.Cfg, siteName, hostName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rec, found, err := sitehosts.Get(ctx, s.Client, s.Cfg.Namespace, siteName, hostName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusAccepted, rec)
}
