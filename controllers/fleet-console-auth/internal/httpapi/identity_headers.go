package httpapi

import (
	"context"
	"net/http"
	"strings"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/rbac"
)

func (s *Server) setFleetIdentityHeaders(r *http.Request, actor rbac.Actor) {
	roles := s.actorRoleNames(r.Context(), actor)
	r.Header.Set("X-Fleet-Subject-Kind", actor.Kind)
	r.Header.Set("X-Fleet-Subject-Name", actor.Name)
	r.Header.Set("X-Fleet-Roles", strings.Join(roles, ","))
	isDC, _ := s.RBAC.IsDCAdmin(r.Context(), actor)
	if isDC {
		r.Header.Set("X-Fleet-DC-Admin", "true")
	}
}

func (s *Server) actorRoleNames(ctx context.Context, actor rbac.Actor) []string {
	_, bindings, err := s.RBAC.LoadPolicy(ctx)
	if err != nil {
		return nil
	}
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
