package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/invites"
)

var tenantNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

type tenantCreateBody struct {
	Name string `json:"name"`
}

type tenantMemberBody struct {
	Username string `json:"username"`
}

type tenantGroupView struct {
	Name    string                 `json:"name"`
	ID      int                    `json:"id"`
	System  bool                   `json:"system"`
	Members []invites.DirectoryUser `json:"members"`
}

// isSystemGroup marks LLDAP-internal groups that must not be deleted here.
func isSystemGroup(name string) bool {
	return strings.HasPrefix(name, "lldap_")
}

func (s *Server) lldapClient(r *http.Request) (*invites.LLDAP, string, error) {
	user, pass, err := s.lldapServiceAccount(r.Context())
	if err != nil {
		return nil, "", err
	}
	l := invites.NewLLDAP(s.Cfg.LldapURL)
	token, err := l.Login(r.Context(), user, pass)
	if err != nil {
		return nil, "", err
	}
	return l, token, nil
}

func (s *Server) handleTenants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listTenants(w, r)
	case http.MethodPost:
		s.createTenant(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listTenants(w http.ResponseWriter, r *http.Request) {
	if !s.requireIdentity(w, r) {
		return
	}
	l, token, err := s.lldapClient(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	groups, err := l.ListGroups(r.Context(), token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	users, err := l.ListUsers(r.Context(), token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	views := make([]tenantGroupView, 0, len(groups))
	for _, g := range groups {
		members := g.Users
		if members == nil {
			members = []invites.DirectoryUser{}
		}
		views = append(views, tenantGroupView{
			Name:    g.DisplayName,
			ID:      g.ID,
			System:  isSystemGroup(g.DisplayName),
			Members: members,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": views, "users": users})
}

func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	if !a.isPlatformAdmin() {
		http.Error(w, "forbidden: platform admin required", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req tenantCreateBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	if !tenantNameRe.MatchString(name) {
		http.Error(w, "invalid group name", http.StatusBadRequest)
		return
	}
	l, token, err := s.lldapClient(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := l.CreateGroup(r.Context(), token, name); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleTenantsSub(w http.ResponseWriter, r *http.Request, sub string) {
	parts := strings.Split(strings.Trim(sub, "/"), "/")
	name := parts[0]
	// {name}/members[/{username}]
	if len(parts) >= 2 && parts[1] == "members" {
		switch {
		case len(parts) == 2 && r.Method == http.MethodPost:
			s.addTenantMember(w, r, name)
		case len(parts) == 3 && r.Method == http.MethodDelete:
			s.removeTenantMember(w, r, name, parts[2])
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		s.deleteTenant(w, r, name)
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func (s *Server) deleteTenant(w http.ResponseWriter, r *http.Request, name string) {
	a, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	if !a.isPlatformAdmin() {
		http.Error(w, "forbidden: platform admin required", http.StatusForbidden)
		return
	}
	if isSystemGroup(name) {
		http.Error(w, "system group cannot be deleted", http.StatusBadRequest)
		return
	}
	l, token, err := s.lldapClient(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	gid, found, err := l.GroupID(r.Context(), token, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !found {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}
	if err := l.DeleteGroup(r.Context(), token, gid); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addTenantMember(w http.ResponseWriter, r *http.Request, name string) {
	a, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	if !a.isPlatformAdmin() {
		http.Error(w, "forbidden: platform admin required", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req tenantMemberBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if !inviteUserRe.MatchString(username) {
		http.Error(w, "invalid username", http.StatusBadRequest)
		return
	}
	l, token, err := s.lldapClient(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	gid, found, err := l.GroupID(r.Context(), token, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !found {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}
	exists, err := l.UserExists(r.Context(), token, username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !exists {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if err := l.AddUserToGroup(r.Context(), token, username, gid); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeTenantMember(w http.ResponseWriter, r *http.Request, name, username string) {
	a, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	if !a.isPlatformAdmin() {
		http.Error(w, "forbidden: platform admin required", http.StatusForbidden)
		return
	}
	username = strings.ToLower(strings.TrimSpace(username))
	l, token, err := s.lldapClient(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	gid, found, err := l.GroupID(r.Context(), token, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !found {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}
	if err := l.RemoveUserFromGroup(r.Context(), token, username, gid); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
