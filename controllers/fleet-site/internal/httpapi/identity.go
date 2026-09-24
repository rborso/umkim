package httpapi

import (
	"net/http"
	"strings"
)

const (
	headerSubjectKind = "X-Fleet-Subject-Kind"
	headerSubjectName = "X-Fleet-Subject-Name"
	headerDCAdmin     = "X-Fleet-DC-Admin"
	headerRoles       = "X-Fleet-Roles"
)

// FleetActor is the console identity forwarded by fleet-console-auth.
type FleetActor struct {
	Kind    string
	Name    string
	Roles   []string
	DCAdmin bool
}

func actorFromRequest(r *http.Request) FleetActor {
	roles := []string{}
	if raw := strings.TrimSpace(r.Header.Get(headerRoles)); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				roles = append(roles, p)
			}
		}
	}
	return FleetActor{
		Kind:    strings.TrimSpace(r.Header.Get(headerSubjectKind)),
		Name:    strings.TrimSpace(r.Header.Get(headerSubjectName)),
		Roles:   roles,
		DCAdmin: strings.EqualFold(r.Header.Get(headerDCAdmin), "true"),
	}
}

func (a FleetActor) isPlatformAdmin() bool {
	return a.DCAdmin
}

// requireIdentity requires console identity headers on sensitive reads such as
// bootstrap script download. Lab mode (RequireIdentity false) stays open.
func (s *Server) requireIdentity(w http.ResponseWriter, r *http.Request) bool {
	a := actorFromRequest(r)
	if a.Kind != "" && a.Name != "" {
		return true
	}
	if s.Cfg.RequireIdentity {
		http.Error(w, "unauthorized: missing fleet identity headers", http.StatusUnauthorized)
		return false
	}
	return true
}

// requireMutatingActor ensures identity headers are present for console-originated mutations.
// When Cfg.RequireIdentity is true (default), missing subject headers return 401.
// When false (lab), missing headers are treated as platform admin for direct API access.
func (s *Server) requireMutatingActor(w http.ResponseWriter, r *http.Request) (FleetActor, bool) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return actorFromRequest(r), true
	}
	// Enrollment and bootstrap token paths bypass this check elsewhere.
	a := actorFromRequest(r)
	if a.Kind == "" || a.Name == "" {
		if s.Cfg.RequireIdentity {
			http.Error(w, "unauthorized: missing fleet identity headers", http.StatusUnauthorized)
			return FleetActor{}, false
		}
		return FleetActor{DCAdmin: true}, true
	}
	return a, true
}
