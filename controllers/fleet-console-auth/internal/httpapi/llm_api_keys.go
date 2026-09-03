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
	APIEndpoint string                  `json:"apiEndpoint,omitempty"`
	Keys        []authv1.FleetLLMApiKey `json:"keys"`
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

func (s *Server) handleLLMApiKeys(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDCAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		keys, err := s.llmKeys().List(r.Context())
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
		token, key, err := s.llmKeys().Create(r.Context(), name, strings.TrimSpace(body.DisplayName), body.Models)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		actor, _ := s.resolveActor(r)
		_ = s.Audit.Record(r.Context(), authorizer.SubjectRefFromActor(actor), "create.llm-api-key", "allow",
			&authv1.ScopeRef{Kind: "llm-api-keys", Name: key.Name}, nil, httpInfo(r), "")
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"key":   key,
			"token": token,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleLLMApiKeyByName(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDCAdmin(w, r); !ok {
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/llm-api-keys/")
	name = strings.Trim(name, "/")
	parts := strings.SplitN(name, "/", 2)
	keyName := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	if action == "rotate" && r.Method == http.MethodPost {
		token, key, err := s.llmKeys().Rotate(r.Context(), keyName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		actor, _ := s.resolveActor(r)
		_ = s.Audit.Record(r.Context(), authorizer.SubjectRefFromActor(actor), "rotate.llm-api-key", "allow",
			&authv1.ScopeRef{Kind: "llm-api-keys", Name: keyName}, nil, httpInfo(r), "")
		writeJSON(w, http.StatusOK, map[string]interface{}{"key": key, "token": token})
		return
	}

	if action == "revoke" && r.Method == http.MethodPost {
		key, err := s.llmKeys().SetActive(r.Context(), keyName, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, key)
		return
	}

	if action == "activate" && r.Method == http.MethodPost {
		key, err := s.llmKeys().SetActive(r.Context(), keyName, true)
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
		key, err := s.llmKeys().Get(r.Context(), keyName)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, key)
	case http.MethodDelete:
		if err := s.llmKeys().Delete(r.Context(), keyName); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		actor, _ := s.resolveActor(r)
		_ = s.Audit.Record(r.Context(), authorizer.SubjectRefFromActor(actor), "delete.llm-api-key", "allow",
			&authv1.ScopeRef{Kind: "llm-api-keys", Name: keyName}, nil, httpInfo(r), "")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
