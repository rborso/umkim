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
)

type ingressAuthResponse struct {
	HubEnabled            bool     `json:"hubEnabled"`
	HubExposeLldapWebUI   bool     `json:"hubExposeLldapWebUI"`
	HubLldapWebUIExposure string   `json:"hubLldapWebUIExposure,omitempty"`
	ConsoleLldapDeployed  bool     `json:"consoleLldapDeployed"`
	PortalHost            string   `json:"portalHost,omitempty"`
	CookieDomain          string   `json:"cookieDomain,omitempty"`
	AccessDomains         []string `json:"accessDomains,omitempty"`
	Phase                 string   `json:"phase,omitempty"`
	Message               string   `json:"message,omitempty"`
	Hub                   struct {
		Phase              string `json:"phase,omitempty"`
		Message            string `json:"message,omitempty"`
		AutheliaServiceURL string `json:"autheliaServiceURL,omitempty"`
		LLDAPWebUIEnabled  bool   `json:"lldapWebUIEnabled,omitempty"`
		LLDAPWebUIURL      string `json:"lldapWebUIURL,omitempty"`
		LLDAPWebUIExposure string `json:"lldapWebUIExposure,omitempty"`
		LLDAPWebUIPhase    string `json:"lldapWebUIPhase,omitempty"`
		LLDAPWebUIMessage  string `json:"lldapWebUIMessage,omitempty"`
	} `json:"hub,omitempty"`
	Clusters    []fleetv1alpha1.FleetIngressAuthClusterStatus `json:"clusters,omitempty"`
	LastJobName string                                        `json:"lastJobName,omitempty"`
}

type ingressAuthPatchBody struct {
	HubEnabled            *bool    `json:"hubEnabled,omitempty"`
	HubExposeLldapWebUI   *bool    `json:"hubExposeLldapWebUI,omitempty"`
	HubLldapWebUIExposure string   `json:"hubLldapWebUIExposure,omitempty"`
	PortalHost            string   `json:"portalHost,omitempty"`
	CookieDomain          string   `json:"cookieDomain,omitempty"`
	AccessDomains         []string `json:"accessDomains,omitempty"`
}

func (s *Server) handleIngressAuth(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetIngressAuth(r.Context(), w)
	case http.MethodPatch:
		s.handlePatchIngressAuth(r.Context(), w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleIngressAuthSub(w http.ResponseWriter, r *http.Request, sub string) {
	if sub == "reconcile" && r.Method == http.MethodPost {
		s.handleForceIngressAuthReconcile(r.Context(), w)
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func (s *Server) handleGetIngressAuth(ctx context.Context, w http.ResponseWriter) {
	fia, err := ingressauth.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	deployed, err := ingressauth.ConsoleLLDAPDeployed(ctx, s.Client, s.Cfg.ConsoleNamespace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ingressAuthToResponse(fia, deployed))
}

func (s *Server) handlePatchIngressAuth(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var patch ingressAuthPatchBody
	if err := json.Unmarshal(body, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fia, err := ingressauth.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if patch.HubEnabled != nil {
		fia.Spec.HubEnabled = *patch.HubEnabled
	}
	if mode := strings.ToLower(strings.TrimSpace(patch.HubLldapWebUIExposure)); mode != "" {
		if !fleetv1alpha1.ValidLldapWebUIExposure(mode) {
			http.Error(w, "hubLldapWebUIExposure must be disabled, wireguard, or authelia", http.StatusBadRequest)
			return
		}
		if mode != fleetv1alpha1.LldapWebUIExposureDisabled {
			deployed, err := ingressauth.ConsoleLLDAPDeployed(ctx, s.Client, s.Cfg.ConsoleNamespace)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !deployed {
				http.Error(w, "console LLDAP is not deployed (enable fleet_auth_lldap_enabled on the hub)", http.StatusConflict)
				return
			}
		}
		fia.Spec.HubLldapWebUIExposure = mode
		fia.Spec.HubExposeLldapWebUI = mode == fleetv1alpha1.LldapWebUIExposureAuthelia
	} else if patch.HubExposeLldapWebUI != nil {
		if *patch.HubExposeLldapWebUI {
			deployed, err := ingressauth.ConsoleLLDAPDeployed(ctx, s.Client, s.Cfg.ConsoleNamespace)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !deployed {
				http.Error(w, "console LLDAP is not deployed (enable fleet_auth_lldap_enabled on the hub)", http.StatusConflict)
				return
			}
		}
		fia.Spec.HubExposeLldapWebUI = *patch.HubExposeLldapWebUI
		fia.Spec.HubLldapWebUIExposure = ""
	}
	if patch.PortalHost != "" {
		fia.Spec.PortalHost = patch.PortalHost
	}
	if patch.CookieDomain != "" {
		fia.Spec.CookieDomain = patch.CookieDomain
	}
	if patch.AccessDomains != nil {
		fia.Spec.AccessDomains = patch.AccessDomains
	}
	if fia.Annotations == nil {
		fia.Annotations = map[string]string{}
	}
	fia.Annotations[ingressauth.ReconcileAnnotation] = time.Now().UTC().Format(time.RFC3339)

	if err := s.Client.Update(ctx, fia); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	deployed, _ := ingressauth.ConsoleLLDAPDeployed(ctx, s.Client, s.Cfg.ConsoleNamespace)
	writeJSON(w, http.StatusOK, ingressAuthToResponse(fia, deployed))
}

func (s *Server) handleForceIngressAuthReconcile(ctx context.Context, w http.ResponseWriter) {
	fia, err := ingressauth.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fia.Annotations == nil {
		fia.Annotations = map[string]string{}
	}
	fia.Annotations[ingressauth.ReconcileAnnotation] = time.Now().UTC().Format(time.RFC3339)
	if err := s.Client.Update(ctx, fia); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	deployed, _ := ingressauth.ConsoleLLDAPDeployed(ctx, s.Client, s.Cfg.ConsoleNamespace)
	writeJSON(w, http.StatusOK, ingressAuthToResponse(fia, deployed))
}

func ingressAuthToResponse(fia *fleetv1alpha1.FleetIngressAuth, consoleLldapDeployed bool) ingressAuthResponse {
	out := ingressAuthResponse{
		HubEnabled:            fia.Spec.HubEnabled,
		HubExposeLldapWebUI:   fia.Spec.HubExposeLldapWebUI,
		HubLldapWebUIExposure: fia.Spec.EffectiveHubLldapWebUIExposure(),
		ConsoleLldapDeployed:  consoleLldapDeployed,
		PortalHost:            fia.Spec.PortalHost,
		CookieDomain:          fia.Spec.CookieDomain,
		AccessDomains:         fia.Spec.AccessDomains,
		Phase:                 fia.Status.Phase,
		Message:               fia.Status.Message,
		Clusters:              fia.Status.Clusters,
		LastJobName:           fia.Status.LastJobName,
	}
	out.Hub.Phase = fia.Status.Hub.Phase
	out.Hub.Message = fia.Status.Hub.Message
	out.Hub.AutheliaServiceURL = fia.Status.Hub.AutheliaServiceURL
	out.Hub.LLDAPWebUIEnabled = fia.Status.Hub.LLDAPWebUIEnabled
	out.Hub.LLDAPWebUIURL = fia.Status.Hub.LLDAPWebUIURL
	out.Hub.LLDAPWebUIExposure = fia.Status.Hub.LLDAPWebUIExposure
	out.Hub.LLDAPWebUIPhase = fia.Status.Hub.LLDAPWebUIPhase
	out.Hub.LLDAPWebUIMessage = fia.Status.Hub.LLDAPWebUIMessage
	return out
}

func (s *Server) patchClusterIngressAuth(ctx context.Context, clusterName string, enabled bool) error {
	fcj, err := s.getFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return apierrors.NewNotFound(fleetv1alpha1.GroupVersion.WithResource("fleetclusterjoins").GroupResource(), clusterName)
		}
		return err
	}
	if enabled {
		return errClusterIngressAuthUseEnable
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		return fmt.Errorf("%w", errClusterNotReady)
	}
	fcj.Spec.IngressAuth.Enabled = false
	fcj.Status.IngressAuth = fleetv1alpha1.FleetClusterJoinIngressAuthStatus{
		Phase:   fleetv1alpha1.IngressAuthClusterPhaseDisabled,
		Message: "Ingress auth disabled",
	}
	if err := s.Client.Update(ctx, fcj); err != nil {
		return err
	}
	if err := s.Client.Status().Update(ctx, fcj); err != nil {
		return err
	}
	return s.triggerIngressAuthReconcile(ctx)
}

var errClusterNotReady = errors.New("cluster must be Ready")
