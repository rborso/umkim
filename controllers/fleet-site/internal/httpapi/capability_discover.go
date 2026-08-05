package httpapi

import (
	"context"
	"net/http"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/capdiscover"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type capabilityDiscoverySummary struct {
	Phase               string                   `json:"phase,omitempty"`
	Message             string                   `json:"message,omitempty"`
	CompletedAt         string                   `json:"completed_at,omitempty"`
	Nodes               []capdiscover.NodeResult `json:"nodes,omitempty"`
	ClusterCapabilities map[string]bool          `json:"cluster_capabilities,omitempty"`
	ClusterResources    map[string]int           `json:"cluster_resources,omitempty"`
}

func discoveryFromMC(mcAnn map[string]string) *capabilityDiscoverySummary {
	r := capdiscover.FromAnnotations(mcAnn)
	if r == nil {
		return nil
	}
	return &capabilityDiscoverySummary{
		Phase:               r.Phase,
		Message:             r.Message,
		CompletedAt:         r.CompletedAt,
		Nodes:               r.Nodes,
		ClusterCapabilities: r.ClusterCapabilities,
		ClusterResources:    r.ClusterResources,
	}
}

func (s *Server) handleDiscoverCapabilities(ctx context.Context, w http.ResponseWriter, clusterName string) {
	mc, err := getManagedCluster(ctx, s.Client, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster not registered in OCM", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if capdiscover.IsRunning(mc.GetAnnotations()) {
		http.Error(w, "capability discovery already running", http.StatusConflict)
		return
	}
	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if s.clusterReadOnly(clusterName, edge) {
		http.Error(w, "cluster is read-only", http.StatusForbidden)
		return
	}

	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster not found (no FleetClusterJoin)", http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		http.Error(w, "cluster must be Ready", http.StatusConflict)
		return
	}

	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	fcj.Annotations["fleet.umkim.com/discover-capabilities"] = time.Now().UTC().Format(time.RFC3339)
	if err := s.Client.Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		mc, err := getManagedCluster(ctx, s.Client, clusterName)
		if err != nil {
			continue
		}
		st := capdiscover.FromAnnotations(mc.GetAnnotations())
		if st == nil {
			continue
		}
		switch st.Phase {
		case capdiscover.PhaseSucceeded, capdiscover.PhaseFailed:
			if st.Phase == capdiscover.PhaseSucceeded {
				_ = s.syncEdgeCapabilities(ctx, clusterName, st.ClusterCapabilities)
			}
			writeJSON(w, http.StatusOK, st)
			return
		}
	}
	http.Error(w, "capability discovery timed out", http.StatusGatewayTimeout)
}

func (s *Server) syncEdgeCapabilities(ctx context.Context, clusterName string, caps map[string]bool) error {
	if len(caps) == 0 {
		return nil
	}
	edgeFile, err := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if err != nil {
		return err
	}
	ent, ok := edgeFile.Clusters[clusterName]
	if !ok {
		return nil
	}
	if ent.Capabilities == nil {
		ent.Capabilities = map[string]interface{}{}
	}
	for k, v := range caps {
		ent.Capabilities[k] = v
	}
	edgeFile.Clusters[clusterName] = ent
	return saveEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName, edgeFile)
}

func clusterHasKVMCapability(ctx context.Context, s *Server, clusterName string) bool {
	mc, err := getManagedCluster(ctx, s.Client, clusterName)
	if err != nil {
		return false
	}
	labels := mcLabels(mc)
	return labels["fleet.capability/kvm"] == "true"
}
