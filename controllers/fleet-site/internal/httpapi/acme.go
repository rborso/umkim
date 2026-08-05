package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/acme"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

type acmeResponse struct {
	HubEnabled  bool                                    `json:"hubEnabled"`
	Staging     bool                                    `json:"staging,omitempty"`
	Email       string                                  `json:"email,omitempty"`
	Phase       string                                  `json:"phase,omitempty"`
	Message     string                                  `json:"message,omitempty"`
	Hub         fleetv1alpha1.FleetAcmeHubStatus        `json:"hub,omitempty"`
	Clusters    []fleetv1alpha1.FleetAcmeClusterStatus  `json:"clusters,omitempty"`
	LastJobName string                                  `json:"lastJobName,omitempty"`
}

type acmePatchBody struct {
	HubEnabled *bool  `json:"hubEnabled,omitempty"`
	Staging    *bool  `json:"staging,omitempty"`
	Email      string `json:"email,omitempty"`
}

func (s *Server) handleAcme(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetAcme(r.Context(), w)
	case http.MethodPatch:
		s.handlePatchAcme(r.Context(), w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAcmeSub(w http.ResponseWriter, r *http.Request, sub string) {
	if sub == "reconcile" && r.Method == http.MethodPost {
		s.handleForceAcmeReconcile(r.Context(), w)
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func (s *Server) handleGetAcme(ctx context.Context, w http.ResponseWriter) {
	fa, err := acme.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, acmeToResponse(fa))
}

func (s *Server) handlePatchAcme(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var patch acmePatchBody
	if err := json.Unmarshal(body, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fa, err := acme.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if patch.HubEnabled != nil {
		fa.Spec.HubEnabled = *patch.HubEnabled
	}
	if patch.Staging != nil {
		fa.Spec.Staging = *patch.Staging
	}
	if patch.Email != "" {
		fa.Spec.Email = patch.Email
	}
	if fa.Annotations == nil {
		fa.Annotations = map[string]string{}
	}
	fa.Annotations[acme.ReconcileAnnotation] = time.Now().UTC().Format(time.RFC3339)

	if err := s.Client.Update(ctx, fa); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, acmeToResponse(fa))
}

func (s *Server) handleForceAcmeReconcile(ctx context.Context, w http.ResponseWriter) {
	if err := s.triggerAcmeReconcile(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fa, err := acme.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, acmeToResponse(fa))
}

func acmeToResponse(fa *fleetv1alpha1.FleetAcme) acmeResponse {
	email := fa.Spec.Email
	if email == "" {
		email = ""
	}
	return acmeResponse{
		HubEnabled:  fa.Spec.HubEnabled,
		Staging:     fa.Spec.Staging,
		Email:       email,
		Phase:       fa.Status.Phase,
		Message:     fa.Status.Message,
		Hub:         fa.Status.Hub,
		Clusters:    fa.Status.Clusters,
		LastJobName: fa.Status.LastJobName,
	}
}

func (s *Server) triggerAcmeReconcile(ctx context.Context) error {
	var fa fleetv1alpha1.FleetAcme
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: fleetv1alpha1.FleetAcmeName}, &fa); err != nil {
		if apierrors.IsNotFound(err) {
			_, err = acme.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg)
			return err
		}
		return err
	}
	if fa.Annotations == nil {
		fa.Annotations = map[string]string{}
	}
	fa.Annotations[acme.ReconcileAnnotation] = time.Now().UTC().Format(time.RFC3339)
	return s.Client.Update(ctx, &fa)
}

var (
	errClusterAcmeNotReady     = errors.New("cluster must be Ready before enabling ACME")
	errClusterAcmeWrongPolicy  = errors.New("cluster TLS policy must be acme_cluster")
	errClusterAcmeNoDirect     = errors.New("cluster requires direct ingress (port_forward or reverse_proxy)")
	errClusterAcmeUseEnable    = errors.New("use POST /clusters/{name}/acme/enable")
)
