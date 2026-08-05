package httpapi

import (
	"context"
	"net/http"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *Server) handleClusterAcme(w http.ResponseWriter, r *http.Request, clusterName, sub string) {
	switch sub {
	case "enable":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleEnableClusterAcme(r.Context(), w, clusterName)
	case "disable":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleDisableClusterAcme(r.Context(), w, clusterName)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleEnableClusterAcme(ctx context.Context, w http.ResponseWriter, clusterName string) {
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster join not found; ACME is only available for console-bootstrapped clusters", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		http.Error(w, errClusterAcmeNotReady.Error(), http.StatusConflict)
		return
	}
	if !s.clusterAcmeEligible(ctx, clusterName, fcj) {
		http.Error(w, errClusterAcmeWrongPolicy.Error(), http.StatusConflict)
		return
	}
	if !s.clusterDirectIngress(ctx, clusterName, fcj) {
		http.Error(w, errClusterAcmeNoDirect.Error(), http.StatusConflict)
		return
	}

	fcj.Spec.Acme.Enabled = true
	fcj.Status.Acme = fleetv1alpha1.FleetClusterJoinAcmeStatus{
		Phase:   fleetv1alpha1.AcmeClusterPhasePending,
		Message: "Cluster ACME enable requested",
	}
	if err := s.Client.Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.Client.Status().Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.triggerAcmeReconcile(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cs, err := s.buildClusterSummary(ctx, clusterName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) handleDisableClusterAcme(ctx context.Context, w http.ResponseWriter, clusterName string) {
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster join not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fcj.Spec.Acme.Enabled = false
	fcj.Status.Acme = fleetv1alpha1.FleetClusterJoinAcmeStatus{
		Phase:   fleetv1alpha1.AcmeClusterPhaseDisabled,
		Message: "Cluster ACME disabled",
	}
	if err := s.Client.Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.Client.Status().Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.triggerAcmeReconcile(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cs, err := s.buildClusterSummary(ctx, clusterName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) clusterAcmeEligible(ctx context.Context, clusterName string, fcj *fleetv1alpha1.FleetClusterJoin) bool {
	tls := fcj.Spec.Edge.TlsPolicy
	edge, err := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if err == nil {
		if e, ok := edge.Clusters[clusterName]; ok && e.TlsPolicy != "" {
			tls = e.TlsPolicy
		}
	}
	if tls == "" || tls == "inherit" {
		var fs fleetv1alpha1.FleetSite
		if err := s.Client.Get(ctx, client.ObjectKey{Namespace: s.Cfg.Namespace, Name: fcj.Spec.SiteId}, &fs); err == nil {
			fs.Spec.NormalizeEdge()
			for _, d := range fs.Spec.Edge.Domains {
				if d.TlsPolicy != "" {
					tls = d.TlsPolicy
					break
				}
			}
		}
	}
	return tls == fleetv1alpha1.TLSPolicyACMECluster
}

func (s *Server) clusterDirectIngress(ctx context.Context, clusterName string, fcj *fleetv1alpha1.FleetClusterJoin) bool {
	edge, err := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if err != nil {
		return false
	}
	if e, ok := edge.Clusters[clusterName]; ok {
		return e.DirectIngress
	}
	return s.clusterIngressEnabled(ctx, clusterName, fcj)
}
