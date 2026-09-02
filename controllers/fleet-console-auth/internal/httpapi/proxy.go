package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/authorizer"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/rbac"
)

func (s *Server) handleProxyFleet(w http.ResponseWriter, r *http.Request) {
	s.proxy(w, r, "fleet", s.Cfg.FleetUpstream)
}

func (s *Server) proxy(w http.ResponseWriter, r *http.Request, backend, upstream string) {
	if strings.TrimSpace(upstream) == "" {
		http.Error(w, backend+" API not available", http.StatusServiceUnavailable)
		return
	}
	actor, ok := s.resolveActor(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	mapped := authorizer.MapRequest(r, backend)
	allowed, err := s.Checker.Allow(r.Context(), actor, mapped)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result := "deny"
	if allowed {
		result = "allow"
	}
	resource := &authv1.ScopeRef{Kind: mapped.Resource, Name: mapped.ScopeName}
	scope := &authv1.ScopeRef{Kind: mapped.ScopeKind, Name: mapped.ScopeName}
	_ = s.Audit.Record(r.Context(), authorizer.SubjectRefFromActor(actor), mapped.Verb+"."+mapped.Resource, result, resource, scope, httpInfo(r), "")
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	target, err := url.Parse(upstream)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	origPath := r.URL.Path
	r.URL.Path = strings.TrimPrefix(origPath, "/proxy/fleet")
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		r.URL.Path = "/api" + strings.TrimPrefix(r.URL.Path, "/api")
	}
	r.Host = target.Host
	s.setFleetIdentityHeaders(r, actor)

	if needsListFilter(backend, mapped) {
		s.proxyFilteredList(w, r, proxy, actor, backend, mapped)
		return
	}
	proxy.ServeHTTP(w, r)
}

func needsListFilter(backend string, mapped authorizer.MappedRequest) bool {
	if mapped.Verb != "list" {
		return false
	}
	switch mapped.Resource {
	case "sites", "clusters", "apps":
		return backend == "fleet"
	default:
		return false
	}
}

func (s *Server) proxyFilteredList(w http.ResponseWriter, r *http.Request, proxy *httputil.ReverseProxy, actor rbac.Actor, backend string, mapped authorizer.MappedRequest) {
	buf := &bytes.Buffer{}
	rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK, body: buf}
	proxy.ServeHTTP(rec, r)
	body := rec.body.Bytes()
	if rec.status == http.StatusOK && len(body) > 0 {
		if filtered, err := s.filterListBody(r.Context(), actor, backend, mapped, body); err == nil {
			body = filtered
		}
	}
	for k, vals := range rec.header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(rec.status)
	_, _ = io.Copy(w, bytes.NewReader(body))
}

func (s *Server) filterListBody(ctx context.Context, actor rbac.Actor, backend string, mapped authorizer.MappedRequest, body []byte) ([]byte, error) {
	isDC, _ := s.RBAC.IsDCAdmin(ctx, actor)
	if isDC {
		return body, nil
	}
	orgScopes := s.actorOrgScopes(ctx, actor)
	switch mapped.Resource {
	case "sites":
		return filterFleetSites(body, orgScopes)
	case "clusters":
		return filterFleetClusters(ctx, s, body, orgScopes)
	default:
		return body, nil
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	header http.Header
	body   *bytes.Buffer
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *responseRecorder) Header() http.Header {
	if r.header == nil {
		r.header = make(http.Header)
	}
	return r.header
}

func filterFleetSites(body []byte, orgScopes []string) ([]byte, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, nil
	}
	items, ok := payload["sites"].([]interface{})
	if !ok {
		return body, nil
	}
	allowed := map[string]struct{}{}
	for _, o := range orgScopes {
		allowed[o] = struct{}{}
	}
	var out []interface{}
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		spec, _ := obj["spec"].(map[string]interface{})
		orgID, _ := spec["orgId"].(string)
		if orgID == "" {
			continue
		}
		if _, ok := allowed[orgID]; ok {
			out = append(out, item)
		}
	}
	payload["sites"] = out
	return json.Marshal(payload)
}

func filterFleetClusters(ctx context.Context, s *Server, body []byte, orgScopes []string) ([]byte, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, nil
	}
	items, ok := payload["clusters"].([]interface{})
	if !ok {
		return body, nil
	}
	allowed := map[string]struct{}{}
	for _, o := range orgScopes {
		allowed[o] = struct{}{}
	}
	var out []interface{}
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := obj["name"].(string)
		if name == "" {
			if meta, ok := obj["metadata"].(map[string]interface{}); ok {
				name, _ = meta["name"].(string)
			}
		}
		org := clusterOrgFromListItem(ctx, s, name, obj)
		if org != "" {
			if _, ok := allowed[org]; ok {
				out = append(out, item)
			}
		}
	}
	payload["clusters"] = out
	return json.Marshal(payload)
}

func clusterOrgFromListItem(ctx context.Context, s *Server, name string, obj map[string]interface{}) string {
	if labels, ok := obj["labels"].(map[string]interface{}); ok {
		if org, ok := labels["fleet.org"].(string); ok && org != "" {
			return org
		}
	}
	if name != "" {
		org, _ := s.Checker.ClusterOrg(ctx, name, "")
		return org
	}
	return ""
}
