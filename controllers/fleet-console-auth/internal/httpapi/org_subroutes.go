package httpapi

import (
	"net/http"
	"strings"
)

func (s *Server) handleOrgSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/orgs/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.Error(w, "org slug required", http.StatusBadRequest)
		return
	}
	parts := strings.SplitN(path, "/", 2)
	orgSlug := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}
	if sub == "" {
		r.URL.Path = "/api/v1alpha1/orgs/" + orgSlug
		s.handleOrgBySlug(w, r)
		return
	}
	if sub == "users" {
		s.handleOrgUsers(w, r, orgSlug)
		return
	}
	if strings.HasPrefix(sub, "users/") {
		s.handleOrgUserByName(w, r, orgSlug, strings.TrimPrefix(sub, "users/"))
		return
	}
	if sub == "role-bindings" {
		s.handleOrgRoleBindings(w, r, orgSlug)
		return
	}
	if strings.HasPrefix(sub, "role-bindings/") {
		s.handleOrgRoleBindingByName(w, r, orgSlug, strings.TrimPrefix(sub, "role-bindings/"))
		return
	}
	if sub == "llm-api-keys" {
		s.handleOrgLLMApiKeys(w, r, orgSlug)
		return
	}
	if strings.HasPrefix(sub, "llm-api-keys/") {
		s.handleOrgLLMApiKeyByName(w, r, orgSlug, strings.TrimPrefix(sub, "llm-api-keys/"))
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}
