package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/fleetllm"
	"github.com/rborso/umkim/controllers/fleet-site/internal/llm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

type llmResponse struct {
	HubEnabled  bool                          `json:"hubEnabled"`
	PublicHost  string                        `json:"publicHost,omitempty"`
	Phase       string                        `json:"phase,omitempty"`
	Message     string                        `json:"message,omitempty"`
	Hub         fleetv1alpha1.FleetLLMHubStatus `json:"hub,omitempty"`
	LastJobName string                        `json:"lastJobName,omitempty"`
	Backends       []fleetv1alpha1.FleetLLMBackend `json:"backends,omitempty"`
	BackendsSource string                          `json:"backendsSource,omitempty"`
}

type llmPatchBody struct {
	HubEnabled *bool                           `json:"hubEnabled,omitempty"`
	PublicHost string                          `json:"publicHost,omitempty"`
	Backends   []fleetv1alpha1.FleetLLMBackend `json:"backends,omitempty"`
}

func (s *Server) handleLLM(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetLLM(r.Context(), w)
	case http.MethodPatch:
		s.handlePatchLLM(r.Context(), w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleLLMSub(w http.ResponseWriter, r *http.Request, sub string) {
	if sub == "reconcile" && r.Method == http.MethodPost {
		s.handleForceLLMReconcile(r.Context(), w, r)
		return
	}
	if sub == "backends/sync-from-apps" && r.Method == http.MethodPost {
		s.handleSyncLLMBackendsFromApps(r.Context(), w, r)
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func (s *Server) handleGetLLM(ctx context.Context, w http.ResponseWriter) {
	fl, err := fleetllm.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, llmToResponse(fl, s.Cfg.HubEdgeDomain))
}

func (s *Server) handlePatchLLM(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var patch llmPatchBody
	if err := json.Unmarshal(body, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fl, err := fleetllm.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if patch.HubEnabled != nil {
		fl.Spec.HubEnabled = *patch.HubEnabled
	}
	if patch.PublicHost != "" {
		fl.Spec.PublicHost = patch.PublicHost
	}
	if patch.Backends != nil {
		fl.Spec.Backends = patch.Backends
		if fl.Annotations == nil {
			fl.Annotations = map[string]string{}
		}
		fl.Annotations[fleetllm.BackendsManualAnnotation] = "true"
	}
	if fl.Annotations == nil {
		fl.Annotations = map[string]string{}
	}
	fl.Annotations[fleetllm.ReconcileAnnotation] = time.Now().UTC().Format(time.RFC3339)

	if err := s.Client.Update(ctx, fl); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, llmToResponse(fl, s.Cfg.HubEdgeDomain))
}

func (s *Server) handleForceLLMReconcile(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	if err := s.triggerLLMReconcile(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fl, err := fleetllm.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, llmToResponse(fl, s.Cfg.HubEdgeDomain))
}

func (s *Server) handleSyncLLMBackendsFromApps(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	fl, err := fleetllm.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	backends, err := llm.SyncBackendsFromFleetApps(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fl.Spec.Backends = backends
	if fl.Annotations == nil {
		fl.Annotations = map[string]string{}
	}
	fl.Annotations[fleetllm.BackendsManualAnnotation] = "false"
	fl.Annotations[fleetllm.ReconcileAnnotation] = time.Now().UTC().Format(time.RFC3339)
	if err := s.Client.Update(ctx, fl); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, llmToResponse(fl, s.Cfg.HubEdgeDomain))
}

func llmToResponse(fl *fleetv1alpha1.FleetLLM, domain string) llmResponse {
	src := "auto"
	if fl.Annotations != nil && fl.Annotations[fleetllm.BackendsManualAnnotation] == "true" {
		src = "manual"
	}
	return llmResponse{
		HubEnabled:     fl.Spec.HubEnabled,
		PublicHost:     fl.Spec.EffectivePublicHost(domain),
		Phase:          fl.Status.Phase,
		Message:        fl.Status.Message,
		Hub:            fl.Status.Hub,
		LastJobName:    fl.Status.LastJobName,
		Backends:       fl.Spec.Backends,
		BackendsSource: src,
	}
}

func (s *Server) triggerLLMReconcile(ctx context.Context) error {
	var fl fleetv1alpha1.FleetLLM
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: fleetv1alpha1.FleetLLMName}, &fl); err != nil {
		if apierrors.IsNotFound(err) {
			_, err = fleetllm.EnsureCR(ctx, s.Client, s.Cfg.Namespace, s.Cfg)
			return err
		}
		return err
	}
	if fl.Annotations == nil {
		fl.Annotations = map[string]string{}
	}
	fl.Annotations[fleetllm.ReconcileAnnotation] = time.Now().UTC().Format(time.RFC3339)
	return s.Client.Update(ctx, &fl)
}
