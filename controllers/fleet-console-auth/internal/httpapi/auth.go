package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/authorizer"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/identity"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/rbac"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ip := clientIP(r)
	if !s.allowLogin(ip) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	var body loginRequest
	if err := jsonDecode(r.Body, &body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	principal, err := s.Auth.Authenticate(r.Context(), body.Username, body.Password)
	if err != nil {
		_ = s.Audit.Record(r.Context(), authv1.SubjectRef{Kind: "User", Name: body.Username}, "auth.login", "deny", nil, nil, httpInfo(r), err.Error())
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	groups := s.RBAC.ResolveFleetGroups(r.Context(), principal.Groups)
	actor := rbac.Actor{Kind: "User", Name: principal.ExternalID, Groups: groups}
	_ = s.ensureUser(r.Context(), principal, groups)
	token, err := s.Sessions.Issue(principal.Username, principal.ExternalID, principal.AuthMethod, "User", groups)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.setSessionCookie(w, token)
	_ = s.Audit.Record(r.Context(), authv1.SubjectRef{Kind: "User", Name: principal.ExternalID}, "auth.login", "allow", nil, nil, httpInfo(r), "")
	perms, _ := s.RBAC.EffectivePermissions(r.Context(), actor)
	isAdmin, _ := s.RBAC.IsDCAdmin(r.Context(), actor)
	roles := s.actorRoleNames(r.Context(), actor)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"kind":        "User",
		"name":        principal.ExternalID,
		"username":    principal.Username,
		"displayName": principal.DisplayName,
		"groups":      groups,
		"permissions": perms,
		"dcAdmin":     isAdmin,
		"roles":       roles,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	actor, ok := s.resolveActor(r)
	if ok {
		_ = s.Audit.Record(r.Context(), authorizer.SubjectRefFromActor(actor), "auth.logout", "allow", nil, nil, httpInfo(r), "")
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.Sessions.CookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.Sessions.Secure(),
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	actor, ok := s.resolveActor(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	perms, _ := s.RBAC.EffectivePermissions(r.Context(), actor)
	isAdmin, _ := s.RBAC.IsDCAdmin(r.Context(), actor)
	roles := s.actorRoleNames(r.Context(), actor)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"kind":        actor.Kind,
		"name":        actor.Name,
		"groups":      actor.Groups,
		"permissions": perms,
		"dcAdmin":     isAdmin,
		"roles":       roles,
	})
}

func (s *Server) handleOIDCConfig(w http.ResponseWriter, r *http.Request) {
	enabled := s.OIDC != nil && s.OIDC.Enabled()
	out := map[string]interface{}{
		"enabled": enabled,
		"issuer":  s.Cfg.OIDCIssuer,
	}
	if enabled {
		out["authorizeUrl"] = s.OIDC.AuthorizeURL("fleet-console")
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if s.OIDC == nil || !s.OIDC.Enabled() {
		http.Error(w, "oidc not configured", http.StatusNotFound)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	_, err := s.OIDC.ExchangeCode(r.Context(), code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotImplemented)
		return
	}
}

func (s *Server) resolveActor(r *http.Request) (rbac.Actor, bool) {
	if s.Cfg.AuthBypass {
		return rbac.Actor{Kind: "User", Name: "bypass", Groups: []string{"dc-admins"}}, true
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		raw := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if strings.HasPrefix(raw, "fleet_sa_") {
			saName, err := s.Tokens.Validate(r.Context(), raw)
			if err != nil {
				return rbac.Actor{}, false
			}
			return s.serviceAccountActor(r.Context(), saName)
		}
		claims, err := s.Sessions.Parse(raw)
		if err == nil {
			return rbac.Actor{Kind: claims.Kind, Name: claims.Sub, Groups: claims.Groups}, true
		}
	}
	if c, err := r.Cookie(s.Sessions.CookieName()); err == nil && c.Value != "" {
		claims, err := s.Sessions.Parse(c.Value)
		if err == nil {
			return rbac.Actor{Kind: claims.Kind, Name: claims.Sub, Groups: claims.Groups}, true
		}
	}
	return rbac.Actor{}, false
}

func (s *Server) serviceAccountActor(ctx context.Context, saName string) (rbac.Actor, bool) {
	sa, err := s.RBAC.LoadServiceAccount(ctx, saName)
	if err != nil {
		return rbac.Actor{}, false
	}
	// Inline SA role bindings are merged into FleetRoleBinding subjects at reconcile/seed time;
	// also bind SA name directly for subject matching.
	_ = sa
	return rbac.Actor{Kind: "ServiceAccount", Name: saName}, true
}

func (s *Server) requireDCAdmin(w http.ResponseWriter, r *http.Request) (rbac.Actor, bool) {
	actor, ok := s.resolveActor(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return actor, false
	}
	isAdmin, err := s.RBAC.IsDCAdmin(r.Context(), actor)
	if err != nil || !isAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		_ = s.Audit.Record(r.Context(), authorizer.SubjectRefFromActor(actor), "authz.admin", "deny", nil, nil, httpInfo(r), "")
		return actor, false
	}
	return actor, true
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.Sessions.CookieName(),
		Value:    token,
		Path:     "/",
		MaxAge:   int(s.Cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.Sessions.Secure(),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) ensureUser(ctx context.Context, p *identity.Principal, groups []string) error {
	name := sanitizeName(p.ExternalID)
	var u authv1.FleetUser
	err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &u)
	now := metav1.Now()
	if err != nil {
		u = authv1.FleetUser{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.Cfg.Namespace},
			Spec: authv1.FleetUserSpec{
				ExternalID:  p.ExternalID,
				DisplayName: p.DisplayName,
				Groups:      groups,
				AuthMethod:  p.AuthMethod,
			},
			Status: authv1.FleetUserStatus{LastLogin: &now},
		}
		return s.Client.Create(ctx, &u)
	}
	u.Spec.DisplayName = p.DisplayName
	u.Spec.Groups = groups
	u.Spec.AuthMethod = p.AuthMethod
	u.Status.LastLogin = &now
	return s.Client.Update(ctx, &u)
}

func sanitizeName(s string) string {
	out := make([]rune, 0, len(s))
	for _, c := range strings.ToLower(s) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			out = append(out, c)
		} else if c == '@' || c == '.' || c == '_' {
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "user"
	}
	return string(out)
}

func (s *Server) allowLogin(ip string) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.loginAttempts == nil {
		s.loginAttempts = map[string][]time.Time{}
	}
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	var kept []time.Time
	for _, t := range s.loginAttempts[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= s.Cfg.LoginRateLimitPerMin {
		s.loginAttempts[ip] = kept
		return false
	}
	s.loginAttempts[ip] = append(kept, now)
	return true
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}

func httpInfo(r *http.Request) *authv1.AuditHTTPInfo {
	return &authv1.AuditHTTPInfo{
		Method:   r.Method,
		Path:     r.URL.Path,
		ClientIP: clientIP(r),
	}
}
