package httpapi

import (
	"context"
	"net/http"
	"strings"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/authorizer"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/llmkeys"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

type llmApiKeyCreateBody struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Models      []string `json:"models"`
}

type llmApiKeyListResponse struct {
	APIEndpoint string                    `json:"apiEndpoint,omitempty"`
	Keys        []authv1.FleetLLMApiKey   `json:"keys"`
}

func (s *Server) llmKeys() *llmkeys.Manager {
	return &llmkeys.Manager{Client: s.Client, Namespace: s.Cfg.Namespace}
}

func (s *Server) llmPublicEndpoint(ctx context.Context) string {
	var fl unstructured.Unstructured
	fl.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "fleet.umkim.com", Version: "v1alpha1", Kind: "FleetLLM",
	})
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: "fleet-llm"}, &fl); err != nil {
		return ""
	}
	host, _, _ := unstructured.NestedString(fl.Object, "spec", "publicHost")
	if host == "" {
		return ""
	}
	return "https://" + host + "/v1"
}

func (s *Server) handleOrgLLMApiKeys(w http.ResponseWriter, r *http.Request, orgSlug string) {
	if _, ok := s.requireOrgAdmin(w, r, orgSlug); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		keys, err := s.llmKeys().List(r.Context(), orgSlug)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, llmApiKeyListResponse{
			APIEndpoint: s.llmPublicEndpoint(r.Context()),
			Keys:        keys,
		})
	case http.MethodPost:
		var body llmApiKeyCreateBody
		if err := jsonDecode(r.Body, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		token, key, err := s.llmKeys().Create(r.Context(), orgSlug, name, strings.TrimSpace(body.DisplayName), body.Models)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		actor, _ := s.resolveActor(r)
		_ = s.Audit.Record(r.Context(), authorizer.SubjectRefFromActor(actor), "create.llm-api-key", "allow",
			&authv1.ScopeRef{Kind: "llm-api-keys", Name: key.Name},
			&authv1.ScopeRef{Kind: scopeKindOrg, Name: orgSlug}, httpInfo(r), "")
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"key":   key,
			"token": token,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOrgLLMApiKeyByName(w http.ResponseWriter, r *http.Request, orgSlug, name string) {
	if _, ok := s.requireOrgAdmin(w, r, orgSlug); !ok {
		return
	}
	name = strings.Trim(name, "/")
	parts := strings.SplitN(name, "/", 2)
	keyName := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	if action == "rotate" && r.Method == http.MethodPost {
		token, key, err := s.llmKeys().Rotate(r.Context(), orgSlug, keyName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		actor, _ := s.resolveActor(r)
		_ = s.Audit.Record(r.Context(), authorizer.SubjectRefFromActor(actor), "rotate.llm-api-key", "allow",
			&authv1.ScopeRef{Kind: "llm-api-keys", Name: keyName},
			&authv1.ScopeRef{Kind: scopeKindOrg, Name: orgSlug}, httpInfo(r), "")
		writeJSON(w, http.StatusOK, map[string]interface{}{"key": key, "token": token})
		return
	}

	if action == "revoke" && r.Method == http.MethodPost {
		key, err := s.llmKeys().SetActive(r.Context(), orgSlug, keyName, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, key)
		return
	}

	if action == "activate" && r.Method == http.MethodPost {
		key, err := s.llmKeys().SetActive(r.Context(), orgSlug, keyName, true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, key)
		return
	}

	if action != "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		key, err := s.llmKeys().Get(r.Context(), orgSlug, keyName)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, key)
	case http.MethodDelete:
		if err := s.llmKeys().Delete(r.Context(), orgSlug, keyName); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		actor, _ := s.resolveActor(r)
		_ = s.Audit.Record(r.Context(), authorizer.SubjectRefFromActor(actor), "delete.llm-api-key", "allow",
			&authv1.ScopeRef{Kind: "llm-api-keys", Name: keyName},
			&authv1.ScopeRef{Kind: scopeKindOrg, Name: orgSlug}, httpInfo(r), "")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
