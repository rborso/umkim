package httpapi

import (
	"net/http"
	"sync"
	"time"

	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/audit"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/authorizer"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/config"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/identity"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/rbac"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/session"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/tokens"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Server struct {
	Client        client.Client
	Cfg           config.Config
	Sessions      *session.Manager
	Auth          *identity.Authenticator
	OIDC          identity.OIDCProvider
	RBAC          *rbac.Engine
	Checker       *authorizer.Checker
	Audit         *audit.Recorder
	Tokens        *tokens.Manager
	loginAttempts map[string][]time.Time
	loginMu       sync.Mutex
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/api/v1alpha1/bootstrap", s.handleBootstrap)
	mux.HandleFunc("/api/v1alpha1/login", s.handleLogin)
	mux.HandleFunc("/api/v1alpha1/logout", s.handleLogout)
	mux.HandleFunc("/api/v1alpha1/me", s.handleMe)
	mux.HandleFunc("/api/v1alpha1/oidc/config", s.handleOIDCConfig)
	mux.HandleFunc("/api/v1alpha1/oidc/callback", s.handleOIDCCallback)
	mux.HandleFunc("/api/v1alpha1/users", s.handleUsers)
	mux.HandleFunc("/api/v1alpha1/users/", s.handleUserByName)
	mux.HandleFunc("/api/v1alpha1/groups", s.handleGroups)
	mux.HandleFunc("/api/v1alpha1/groups/", s.handleGroupByName)
	mux.HandleFunc("/api/v1alpha1/role-bindings", s.handleRoleBindings)
	mux.HandleFunc("/api/v1alpha1/role-bindings/", s.handleRoleBindingByName)
	mux.HandleFunc("/api/v1alpha1/service-accounts", s.handleServiceAccounts)
	mux.HandleFunc("/api/v1alpha1/service-accounts/", s.handleServiceAccountByName)
	mux.HandleFunc("/api/v1alpha1/orgs", s.handleOrgs)
	mux.HandleFunc("/api/v1alpha1/orgs/", s.handleOrgSubroutes)
	mux.HandleFunc("/api/v1alpha1/audit-events", s.handleAuditEvents)
	mux.HandleFunc("/proxy/fleet/", s.handleProxyFleet)
	mux.HandleFunc("/proxy/llm/", s.handleProxyLLM)
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = jsonEncode(w, v)
}
