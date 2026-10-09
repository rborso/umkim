package httpapi

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/authorizer"
)

func (s *Server) handleProxyFleet(w http.ResponseWriter, r *http.Request) {
	if isPublicFleetPath(r.URL.Path, r.Method) {
		s.proxyPublic(w, r, s.Cfg.FleetUpstream)
		return
	}
	s.proxy(w, r, "fleet", s.Cfg.FleetUpstream)
}

// isPublicFleetPath reports whether a fleet API path is reachable without a
// console session. Invite redemption is intentionally unauthenticated: the
// one-time token is the credential.
func isPublicFleetPath(path, method string) bool {
	if method != http.MethodPost {
		return false
	}
	p := strings.TrimPrefix(path, "/proxy/fleet")
	return p == "/api/v1alpha1/invites/redeem" || p == "/api/v1alpha1/invites/redeem/"
}

func (s *Server) proxyPublic(w http.ResponseWriter, r *http.Request, upstream string) {
	if strings.TrimSpace(upstream) == "" {
		http.Error(w, "fleet API not available", http.StatusServiceUnavailable)
		return
	}
	target, err := url.Parse(upstream)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/proxy/fleet")
	r.Host = target.Host
	proxy.ServeHTTP(w, r)
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
	// Negative flush interval keeps streaming and websocket upgrades responsive.
	proxy.FlushInterval = -1
	origPath := r.URL.Path
	r.URL.Path = strings.TrimPrefix(origPath, "/proxy/fleet")
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		r.URL.Path = "/api" + strings.TrimPrefix(r.URL.Path, "/api")
	}
	r.Host = target.Host
	s.setFleetIdentityHeaders(r, actor)
	proxy.ServeHTTP(w, r)
}
