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
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/orgpolicy"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/quota"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/rbac"
)

func (s *Server) handleProxyDCM(w http.ResponseWriter, r *http.Request) {
	s.proxy(w, r, "dcm", s.Cfg.DCMUpstream)
}

func (s *Server) handleProxyFleet(w http.ResponseWriter, r *http.Request) {
	s.proxy(w, r, "fleet", s.Cfg.FleetUpstream)
}

func (s *Server) proxy(w http.ResponseWriter, r *http.Request, backend, upstream string) {
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
	if mapped.Resource == "catalog-items" && mapped.ScopeName != "" {
		if mapped.Verb == "get" || mapped.Verb == "list" {
			allowed, _ = s.CatalogACL.CanRead(r.Context(), actor, mapped.ScopeName)
		} else {
			allowed, _ = s.CatalogACL.CanWrite(r.Context(), actor, mapped.ScopeName)
		}
	}
	if mapped.Resource == "catalog-items" && mapped.Verb == "list" {
		allowed = true
	}
	// Block catalog-item mutations for non–dc-admin.
	if mapped.Resource == "catalog-items" && (mapped.Verb == "create" || mapped.Verb == "update" || mapped.Verb == "delete") {
		if isDC, _ := s.RBAC.IsDCAdmin(r.Context(), actor); !isDC {
			allowed = false
		}
	}
	if mapped.Resource == "policies" {
		okPolicy, errMsg, status := s.authorizePolicy(r, actor, mapped)
		if !okPolicy {
			_ = s.Audit.Record(r.Context(), authorizer.SubjectRefFromActor(actor), mapped.Verb+".policies", "deny",
				&authv1.ScopeRef{Kind: "policies", Name: mapped.ScopeName}, nil, httpInfo(r), errMsg)
			http.Error(w, errMsg, status)
			return
		}
		allowed = true
	}
	if mapped.Resource == "providers" && mapped.Verb != "get" && mapped.Verb != "list" {
		allowed = false
	}
	if mapped.Resource == "catalog-item-instances" && mapped.Verb == "create" {
		if err := s.checkInstanceQuota(r, actor); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
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
	if backend == "dcm" {
		r.URL.Path = strings.TrimPrefix(origPath, "/proxy/dcm")
	} else {
		r.URL.Path = strings.TrimPrefix(origPath, "/proxy/fleet")
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		r.URL.Path = "/api" + strings.TrimPrefix(r.URL.Path, "/api")
	}
	r.Host = target.Host
	s.setFleetIdentityHeaders(r, actor)

	if mapped.Resource == "catalog-items" && mapped.Verb == "list" {
		s.proxyCatalogList(w, r, proxy, actor)
		return
	}
	if needsListFilter(backend, mapped) {
		s.proxyFilteredList(w, r, proxy, actor, backend, mapped)
		return
	}
	if mapped.Resource == "catalog-item-instances" && mapped.Verb == "create" {
		s.proxyWithQuotaUpdate(w, r, proxy, actor)
		return
	}
	proxy.ServeHTTP(w, r)
}

func needsListFilter(backend string, mapped authorizer.MappedRequest) bool {
	if mapped.Verb != "list" {
		return false
	}
	switch mapped.Resource {
	case "sites", "clusters":
		return backend == "fleet"
	case "catalog-item-instances", "policies":
		return backend == "dcm"
	default:
		return false
	}
}

func (s *Server) proxyWithQuotaUpdate(w http.ResponseWriter, r *http.Request, proxy *httputil.ReverseProxy, actor rbac.Actor) {
	buf := &bytes.Buffer{}
	rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK, body: buf}
	proxy.ServeHTTP(rec, r)
	if rec.status == http.StatusCreated || rec.status == http.StatusOK {
		for _, org := range s.actorOrgScopes(r.Context(), actor) {
			_ = quota.UpdateOrgUsage(r.Context(), s.Client, s.Cfg.Namespace, s.Cfg.DCMUpstream, org)
		}
	}
	for k, vals := range rec.header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(rec.status)
	_, _ = io.Copy(w, bytes.NewReader(rec.body.Bytes()))
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
	case "catalog-item-instances":
		return filterCatalogInstances(body, orgScopes)
	case "policies":
		return filterPolicies(body, orgScopes, s.actorRoleNames(ctx, actor))
	default:
		return body, nil
	}
}

func (s *Server) authorizePolicy(r *http.Request, actor rbac.Actor, mapped authorizer.MappedRequest) (ok bool, msg string, status int) {
	ctx := r.Context()
	if isDC, _ := s.RBAC.IsDCAdmin(ctx, actor); isDC {
		return true, "", http.StatusOK
	}
	isOrgAdmin := false
	for _, role := range s.actorRoleNames(ctx, actor) {
		if role == roleOrgAdmin {
			isOrgAdmin = true
			break
		}
	}
	orgScopes := s.actorOrgScopes(ctx, actor)
	switch mapped.Verb {
	case "list":
		if isOrgAdmin {
			return true, "", http.StatusOK
		}
		// Customers: allow through so filter returns empty list (Checker may already deny).
		return true, "", http.StatusOK
	case "get":
		if !isOrgAdmin {
			return false, "forbidden", http.StatusForbidden
		}
		if len(orgScopes) == 0 {
			return false, "forbidden", http.StatusForbidden
		}
		for _, org := range orgScopes {
			if err := orgpolicy.ValidateMutation(org, mapped.ScopeName, nil); err == nil {
				return true, "", http.StatusOK
			}
		}
		return false, "forbidden policy", http.StatusForbidden
	case "create", "update", "delete", "patch":
		if !isOrgAdmin {
			return false, "forbidden", http.StatusForbidden
		}
		if len(orgScopes) != 1 {
			return false, "org-admin must have exactly one org scope", http.StatusForbidden
		}
		org := orgScopes[0]
		policyID := mapped.ScopeName
		var body []byte
		if mapped.Verb != "delete" {
			var err error
			body, err = io.ReadAll(r.Body)
			if err != nil {
				return false, err.Error(), http.StatusBadRequest
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			if policyID == "" {
				policyID = policyIDFromCreate(r, body)
			}
		}
		if err := orgpolicy.ValidateMutation(org, policyID, body); err != nil {
			return false, err.Error(), http.StatusForbidden
		}
		return true, "", http.StatusOK
	default:
		return false, "forbidden", http.StatusForbidden
	}
}

func policyIDFromCreate(r *http.Request, body []byte) string {
	if id := r.URL.Query().Get("id"); id != "" {
		return id
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, k := range []string{"id", "name"} {
		if v, ok := payload[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func filterPolicies(body []byte, orgScopes []string, roles []string) ([]byte, error) {
	isOrgAdmin := false
	for _, r := range roles {
		if r == roleOrgAdmin {
			isOrgAdmin = true
			break
		}
	}
	if !isOrgAdmin || len(orgScopes) == 0 {
		return []byte(`{"results":[]}`), nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, nil
	}
	results, ok := payload["results"].([]interface{})
	if !ok {
		return body, nil
	}
	allowedOrgs := map[string]struct{}{}
	for _, o := range orgScopes {
		allowedOrgs[o] = struct{}{}
	}
	var out []interface{}
	for _, item := range results {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		for org := range allowedOrgs {
			if orgpolicy.BelongsToOrg(org, obj) {
				out = append(out, item)
				break
			}
		}
	}
	payload["results"] = out
	return json.Marshal(payload)
}

func (s *Server) checkInstanceQuota(r *http.Request, actor rbac.Actor) error {
	if isDC, _ := s.RBAC.IsDCAdmin(r.Context(), actor); isDC {
		return nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return quota.CheckCreate(r.Context(), s.Client, s.Cfg.Namespace, s.Cfg.DCMUpstream, body, s.actorOrgScopes(r.Context(), actor))
}

func (s *Server) proxyCatalogList(w http.ResponseWriter, r *http.Request, proxy *httputil.ReverseProxy, actor rbac.Actor) {
	buf := &bytes.Buffer{}
	rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK, body: buf}
	proxy.ServeHTTP(rec, r)
	body := rec.body.Bytes()
	if rec.status == http.StatusOK && len(body) > 0 {
		if filtered, err := s.CatalogACL.FilterList(r.Context(), actor, body); err == nil {
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

func filterCatalogInstances(body []byte, orgScopes []string) ([]byte, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, nil
	}
	results, ok := payload["results"].([]interface{})
	if !ok {
		return body, nil
	}
	allowed := map[string]struct{}{}
	for _, o := range orgScopes {
		allowed[o] = struct{}{}
	}
	var out []interface{}
	for _, item := range results {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		org := instanceOrg(obj)
		if org != "" {
			if _, ok := allowed[org]; ok {
				out = append(out, item)
			}
		}
	}
	payload["results"] = out
	return json.Marshal(payload)
}

func instanceOrg(obj map[string]interface{}) string {
	if ls, ok := obj["label_selector"].(map[string]interface{}); ok {
		if org, ok := ls["fleet.org"].(string); ok {
			return org
		}
	}
	if spec, ok := obj["spec"].(map[string]interface{}); ok {
		if ls, ok := spec["label_selector"].(map[string]interface{}); ok {
			if org, ok := ls["fleet.org"].(string); ok {
				return org
			}
		}
	}
	return ""
}
