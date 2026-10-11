package httpapi

import (
	"context"
	"net/http"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/clusterbootstrap"
)

// handleBootstrapCluster runs the cluster bootstrap bundle on the first control
// node via the site jumphost. This is the server-side equivalent of
// `umkim cluster bootstrap`, so a client can bring a cluster up without a
// separate manual bundle run.
func (s *Server) handleBootstrapCluster(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	fcj, err := s.getFleetClusterJoin(ctx, name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if fcj.Status.Phase == fleetv1alpha1.JoinPhaseReady {
		http.Error(w, "cluster is already provisioned", http.StatusConflict)
		return
	}
	job, err := clusterbootstrap.JobForCluster(ctx, s.Client, s.Cfg, fcj)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := clusterbootstrap.EnsureJob(ctx, s.Client, job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"action": "bootstrap", "job": job.Name})
}
