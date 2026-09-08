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
