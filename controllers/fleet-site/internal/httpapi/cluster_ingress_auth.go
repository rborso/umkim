package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingressauth"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

type clusterIngressAuthEnableBody struct {
	LLDAPAdminPassword string `json:"lldapAdminPassword"`
	ExposeLLDAPWebUI   bool   `json:"exposeLldapWebUI"`
}

var (
	errClusterIngressAuthNotReady    = errors.New("cluster must be Ready before enabling ingress auth")
	errClusterIngressDisabled        = errors.New("ingress auth requires an ingress with a DNS zone (port_forward, or reverse_proxy with a zone)")
	errClusterIngressAuthPassword    = errors.New("lldapAdminPassword is required")
	errClusterIngressAuthUseEnable   = errors.New("use POST /clusters/{name}/ingress-auth/enable with lldapAdminPassword")
)

func (s *Server) handleClusterIngressAuth(w http.ResponseWriter, r *http.Request, clusterName, sub string) {
	switch sub {
	case "enable":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleEnableClusterIngressAuth(r.Context(), w, r, clusterName)
	case "disable":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleDisableClusterIngressAuth(r.Context(), w, clusterName)
	case "":
		http.Error(w, "not found", http.StatusNotFound)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleEnableClusterIngressAuth(ctx context.Context, w http.ResponseWriter, r *http.Request, clusterName string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req clusterIngressAuthEnableBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.LLDAPAdminPassword) == "" {
		http.Error(w, errClusterIngressAuthPassword.Error(), http.StatusBadRequest)
		return
	}
	if len(req.LLDAPAdminPassword) < ingressauth.MinLLDAPAdminPasswordLen {
		http.Error(w, fmt.Sprintf("lldapAdminPassword must be at least %d characters", ingressauth.MinLLDAPAdminPasswordLen), http.StatusBadRequest)
		return
	}

	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster join not found; ingress auth is only available for console-bootstrapped clusters", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		http.Error(w, errClusterIngressAuthNotReady.Error(), http.StatusConflict)
		return
	}
	if !s.clusterIngressEnabled(ctx, clusterName, fcj) {
		http.Error(w, errClusterIngressDisabled.Error(), http.StatusConflict)
		return
	}
	dns, err := ingressauth.ResolveClusterDNS(ctx, s.Client, s.Cfg, clusterName, fcj)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	secretRef, err := ingressauth.EnsureClusterSecret(ctx, s.Client, s.Cfg.Namespace, clusterName, req.LLDAPAdminPassword)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fcj.Spec.IngressAuth.Enabled = true
	fcj.Spec.IngressAuth.ExposeLLDAPWebUI = req.ExposeLLDAPWebUI
	status := fleetv1alpha1.FleetClusterJoinIngressAuthStatus{
		Phase:         fleetv1alpha1.IngressAuthClusterPhasePending,
		Message:       "Ingress auth enable requested",
		SecretsRef:    secretRef,
		PortalHost:    dns.PortalHost,
		AccessDomains: dns.AccessDomains,
	}
	if req.ExposeLLDAPWebUI {
		status.LLDAPWebUIHost = dns.LLDAPWebUIHost
	}
	fcj.Status.IngressAuth = status
	if err := s.Client.Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.Client.Status().Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.triggerIngressAuthReconcile(ctx); err != nil {
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

func (s *Server) handleDisableClusterIngressAuth(ctx context.Context, w http.ResponseWriter, clusterName string) {
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster join not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fcj.Spec.IngressAuth.Enabled = false
	fcj.Status.IngressAuth = fleetv1alpha1.FleetClusterJoinIngressAuthStatus{
		Phase:   fleetv1alpha1.IngressAuthClusterPhaseDisabled,
		Message: "Ingress auth disabled",
	}
	if err := s.Client.Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.Client.Status().Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.triggerIngressAuthReconcile(ctx); err != nil {
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

func (s *Server) triggerIngressAuthReconcile(ctx context.Context) error {
	var fia fleetv1alpha1.FleetIngressAuth
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: fleetv1alpha1.FleetIngressAuthName}, &fia); err != nil {
		if apierrors.IsNotFound(err) {
			_, err = ingressauth.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg)
			return err
		}
		return err
	}
	if fia.Annotations == nil {
		fia.Annotations = map[string]string{}
	}
	fia.Annotations[ingressauth.ReconcileAnnotation] = time.Now().UTC().Format(time.RFC3339)
	return s.Client.Update(ctx, &fia)
}

func (s *Server) clusterIngressEnabled(ctx context.Context, clusterName string, fcj *fleetv1alpha1.FleetClusterJoin) bool {
	ingressID := strings.TrimSpace(fcj.Spec.Edge.IngressId)
	edge, err := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if err == nil {
		if e, ok := edge.Clusters[clusterName]; ok && e.IngressID != "" {
			ingressID = e.IngressID
		}
	}
	if ingressID == "" {
		return false
	}
	site, err := s.loadSiteForCluster(ctx, fcj.Spec.SiteId)
	if err != nil {
		return false
	}
	return clusterIngressAllowsAuth(site, ingressID)
}

func (s *Server) buildClusterSummary(ctx context.Context, name string) (clusterSummary, error) {
	mc, err := getManagedCluster(ctx, s.Client, name)
	if err != nil {
		return clusterSummary{}, err
	}
	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	cs := clusterFromMC(mc, edge)
	if fcj, jerr := s.findFleetClusterJoin(ctx, name); jerr == nil {
		applyFleetClusterJoinSummary(&cs, fcj)
	}
	return cs, nil
}
