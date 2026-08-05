package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/authorizer"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/rbac"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	roleCustomer  = "customer"
	roleOrgAdmin  = "org-admin"
	roleDCAdmin   = "dc-admin"
	scopeKindOrg  = "Org"
)

func (s *Server) requireOrgAdmin(w http.ResponseWriter, r *http.Request, orgSlug string) (rbac.Actor, bool) {
	actor, ok := s.resolveActor(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return actor, false
	}
	if isDC, _ := s.RBAC.IsDCAdmin(r.Context(), actor); isDC {
		return actor, true
	}
	allowed, err := s.RBAC.Authorize(r.Context(), actor, rbac.Request{
		Resource: "users", Verb: "list", ScopeKind: scopeKindOrg, ScopeName: orgSlug,
	})
	if err != nil || !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return actor, false
	}
	return actor, true
}

func (s *Server) actorOrgScopes(ctx context.Context, actor rbac.Actor) []string {
	if isDC, _ := s.RBAC.IsDCAdmin(ctx, actor); isDC {
		return nil
	}
	perms, err := s.RBAC.EffectivePermissions(ctx, actor)
	if err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	for _, p := range perms {
		if strings.EqualFold(p.ScopeKind, scopeKindOrg) && p.ScopeName != "" && p.ScopeName != "*" {
			seen[p.ScopeName] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for slug := range seen {
		out = append(out, slug)
	}
	return out
}

func (s *Server) actorRoleNames(ctx context.Context, actor rbac.Actor) []string {
	_, bindings, err := s.RBAC.LoadPolicy(ctx)
	if err != nil {
		return nil
	}
	roles, _, err := s.RBAC.LoadPolicy(ctx)
	if err != nil {
		return nil
	}
	_ = roles
	seen := map[string]struct{}{}
	for _, b := range bindings {
		if !subjectMatchesActor(actor, b.Spec.Subjects) {
			continue
		}
		if b.Spec.RoleRef.Name != "" {
			seen[b.Spec.RoleRef.Name] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	return out
}

func subjectMatchesActor(actor rbac.Actor, subjects []authv1.SubjectRef) bool {
	for _, sub := range subjects {
		switch sub.Kind {
		case "User":
			if actor.Kind == "User" && actor.Name == sub.Name {
				return true
			}
		case "Group":
			for _, g := range actor.Groups {
				if g == sub.Name {
					return true
				}
			}
		case "ServiceAccount":
			if actor.Kind == "ServiceAccount" && actor.Name == sub.Name {
				return true
			}
		}
	}
	return false
}

func validateOrgRoleBinding(rb *authv1.FleetRoleBinding, orgSlug string) error {
	if rb.Spec.RoleRef.Name == roleDCAdmin {
		return fmt.Errorf("cannot bind dc-admin role")
	}
	for _, sc := range rb.Spec.Scopes {
		if strings.EqualFold(sc.Kind, scopeKindOrg) && sc.Name != orgSlug && sc.Name != "*" {
			return fmt.Errorf("role binding scope must be Org/%s", orgSlug)
		}
	}
	return nil
}

func (s *Server) handleOrgUsers(w http.ResponseWriter, r *http.Request, orgSlug string) {
	if _, ok := s.requireOrgAdmin(w, r, orgSlug); !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var list authv1.FleetUserList
	_ = s.Client.List(r.Context(), &list, client.InNamespace(s.Cfg.Namespace))
	var filtered []authv1.FleetUser
	for _, u := range list.Items {
		if userInOrg(r.Context(), s, u.Name, orgSlug) {
			filtered = append(filtered, u)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"users": filtered})
}

func userInOrg(ctx context.Context, s *Server, userName, orgSlug string) bool {
	var list authv1.FleetRoleBindingList
	if err := s.Client.List(ctx, &list, client.InNamespace(s.Cfg.Namespace)); err != nil {
		return false
	}
	for _, rb := range list.Items {
		for _, sub := range rb.Spec.Subjects {
			if sub.Kind == "User" && sub.Name == userName {
				for _, sc := range rb.Spec.Scopes {
					if strings.EqualFold(sc.Kind, scopeKindOrg) && sc.Name == orgSlug {
						return true
					}
				}
			}
		}
	}
	return false
}

func (s *Server) handleOrgUserByName(w http.ResponseWriter, r *http.Request, orgSlug, userName string) {
	userName = strings.Trim(userName, "/")
	if _, ok := s.requireOrgAdmin(w, r, orgSlug); !ok {
		return
	}
	var u authv1.FleetUser
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: userName}, &u); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, u)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOrgRoleBindings(w http.ResponseWriter, r *http.Request, orgSlug string) {
	if _, ok := s.requireOrgAdmin(w, r, orgSlug); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		var list authv1.FleetRoleBindingList
		_ = s.Client.List(r.Context(), &list, client.InNamespace(s.Cfg.Namespace))
		var filtered []authv1.FleetRoleBinding
		for _, rb := range list.Items {
			if bindingInOrg(rb, orgSlug) {
				filtered = append(filtered, rb)
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"roleBindings": filtered})
	case http.MethodPost:
		var rb authv1.FleetRoleBinding
		if err := jsonDecode(r.Body, &rb); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if rb.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if len(rb.Spec.Scopes) == 0 {
			rb.Spec.Scopes = []authv1.ScopeRef{{Kind: scopeKindOrg, Name: orgSlug}}
		}
		if err := validateOrgRoleBinding(&rb, orgSlug); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rb.Namespace = s.Cfg.Namespace
		if err := s.Client.Create(r.Context(), &rb); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		actor, _ := s.resolveActor(r)
		_ = s.Audit.Record(r.Context(), authorizer.SubjectRefFromActor(actor), "create.role-binding", "allow",
			&authv1.ScopeRef{Kind: "role-bindings", Name: rb.Name},
			&authv1.ScopeRef{Kind: scopeKindOrg, Name: orgSlug}, httpInfo(r), "")
		writeJSON(w, http.StatusCreated, rb)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOrgRoleBindingByName(w http.ResponseWriter, r *http.Request, orgSlug, name string) {
	name = strings.Trim(name, "/")
	if _, ok := s.requireOrgAdmin(w, r, orgSlug); !ok {
		return
	}
	var rb authv1.FleetRoleBinding
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &rb); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !bindingInOrg(rb, orgSlug) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, rb)
	case http.MethodDelete:
		_ = s.Client.Delete(r.Context(), &rb)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func bindingInOrg(rb authv1.FleetRoleBinding, orgSlug string) bool {
	for _, sc := range rb.Spec.Scopes {
		if strings.EqualFold(sc.Kind, scopeKindOrg) && sc.Name == orgSlug {
			return true
		}
	}
	return false
}

func (s *Server) setFleetIdentityHeaders(r *http.Request, actor rbac.Actor) {
	orgScopes := s.actorOrgScopes(r.Context(), actor)
	roles := s.actorRoleNames(r.Context(), actor)
	r.Header.Set("X-Fleet-Subject-Kind", actor.Kind)
	r.Header.Set("X-Fleet-Subject-Name", actor.Name)
	r.Header.Set("X-Fleet-Org-Scopes", strings.Join(orgScopes, ","))
	r.Header.Set("X-Fleet-Roles", strings.Join(roles, ","))
	isDC, _ := s.RBAC.IsDCAdmin(r.Context(), actor)
	if isDC {
		r.Header.Set("X-Fleet-DC-Admin", "true")
	}
}
