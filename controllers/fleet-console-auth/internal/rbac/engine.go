package rbac

import (
	"context"
	"strings"
	"sync"
	"time"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const policyCacheTTL = 30 * time.Second

type policyCache struct {
	mu       sync.RWMutex
	loadedAt time.Time
	roles    map[string]authv1.FleetRole
	bindings []authv1.FleetRoleBinding
}

type Engine struct {
	Client    client.Client
	Namespace string
	cache     policyCache
}

type Actor struct {
	Kind   string
	Name   string
	Groups []string
}

type Request struct {
	Resource    string
	Verb        string
	ScopeKind   string
	ScopeName   string
	AccessLevel string
}

type EffectivePermission struct {
	Resource    string `json:"resource"`
	Verb        string `json:"verb"`
	ScopeKind   string `json:"scopeKind,omitempty"`
	ScopeName   string `json:"scopeName,omitempty"`
	AccessLevel string `json:"accessLevel,omitempty"`
}

func (e *Engine) Authorize(ctx context.Context, actor Actor, req Request) (bool, error) {
	roles, bindings, err := e.loadPolicy(ctx)
	if err != nil {
		return false, err
	}
	for _, b := range bindings {
		if !subjectMatches(actor, b.Spec.Subjects) {
			continue
		}
		role, ok := roles[b.Spec.RoleRef.Name]
		if !ok {
			continue
		}
		if !scopeMatches(b.Spec.Scopes, req.ScopeKind, req.ScopeName, req.Verb) {
			continue
		}
		if ruleAllows(role.Spec.Rules, req) {
			return true, nil
		}
	}
	return false, nil
}

func (e *Engine) EffectivePermissions(ctx context.Context, actor Actor) ([]EffectivePermission, error) {
	roles, bindings, err := e.loadPolicy(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var out []EffectivePermission
	for _, b := range bindings {
		if !subjectMatches(actor, b.Spec.Subjects) {
			continue
		}
		role, ok := roles[b.Spec.RoleRef.Name]
		if !ok {
			continue
		}
		for _, scope := range expandScopes(b.Spec.Scopes) {
			for _, rule := range role.Spec.Rules {
				for _, res := range rule.Resources {
					for _, verb := range rule.Verbs {
						key := res + "|" + verb + "|" + scope.Kind + "|" + scope.Name + "|" + rule.AccessLevel
						if _, dup := seen[key]; dup {
							continue
						}
						seen[key] = struct{}{}
						out = append(out, EffectivePermission{
							Resource:    res,
							Verb:        verb,
							ScopeKind:   scope.Kind,
							ScopeName:   scope.Name,
							AccessLevel: rule.AccessLevel,
						})
					}
				}
			}
		}
	}
	return out, nil
}

func (e *Engine) IsDCAdmin(ctx context.Context, actor Actor) (bool, error) {
	return e.Authorize(ctx, actor, Request{Resource: "*", Verb: "*", ScopeKind: "*", ScopeName: "*"})
}

// LoadPolicy exposes roles and bindings for scope helpers.
func (e *Engine) LoadPolicy(ctx context.Context) (map[string]authv1.FleetRole, []authv1.FleetRoleBinding, error) {
	return e.loadPolicy(ctx)
}

func (e *Engine) loadPolicy(ctx context.Context) (map[string]authv1.FleetRole, []authv1.FleetRoleBinding, error) {
	e.cache.mu.RLock()
	if time.Since(e.cache.loadedAt) < policyCacheTTL && e.cache.roles != nil {
		roles := e.cache.roles
		bindings := e.cache.bindings
		e.cache.mu.RUnlock()
		return roles, bindings, nil
	}
	e.cache.mu.RUnlock()

	var roleList authv1.FleetRoleList
	if err := e.Client.List(ctx, &roleList, client.InNamespace(e.Namespace)); err != nil {
		return nil, nil, err
	}
	roles := map[string]authv1.FleetRole{}
	for _, r := range roleList.Items {
		roles[r.Name] = r
	}
	var bindingList authv1.FleetRoleBindingList
	if err := e.Client.List(ctx, &bindingList, client.InNamespace(e.Namespace)); err != nil {
		return nil, nil, err
	}
	bindings := bindingList.Items

	e.cache.mu.Lock()
	e.cache.loadedAt = time.Now()
	e.cache.roles = roles
	e.cache.bindings = bindings
	e.cache.mu.Unlock()
	return roles, bindings, nil
}

func subjectMatches(actor Actor, subjects []authv1.SubjectRef) bool {
	for _, s := range subjects {
		switch s.Kind {
		case "User":
			if actor.Kind == "User" && actor.Name == s.Name {
				return true
			}
		case "Group":
			for _, g := range actor.Groups {
				if g == s.Name {
					return true
				}
			}
		case "ServiceAccount":
			if actor.Kind == "ServiceAccount" && actor.Name == s.Name {
				return true
			}
		}
	}
	return false
}

func scopeMatches(scopes []authv1.ScopeRef, kind, name, verb string) bool {
	if len(scopes) == 0 {
		return true
	}
	for _, s := range scopes {
		if s.Kind == "*" || s.Name == "*" {
			return true
		}
	}
	if kind == "" && name == "" {
		// Lists stay visible. Unscoped mutations require a global or wildcard binding.
		return verb == "get" || verb == "list"
	}
	for _, s := range scopes {
		if strings.EqualFold(s.Kind, kind) && (s.Name == name || s.Name == "*") {
			return true
		}
	}
	return false
}

func expandScopes(scopes []authv1.ScopeRef) []authv1.ScopeRef {
	if len(scopes) == 0 {
		return []authv1.ScopeRef{{Kind: "*", Name: "*"}}
	}
	return scopes
}

func ruleAllows(rules []authv1.PolicyRule, req Request) bool {
	for _, rule := range rules {
		if !matchResource(rule.Resources, req.Resource) {
			continue
		}
		if !matchVerb(rule.Verbs, req.Verb) {
			continue
		}
		if req.AccessLevel != "" && rule.AccessLevel != "" && rule.AccessLevel != req.AccessLevel && rule.AccessLevel != "*" {
			continue
		}
		return true
	}
	return false
}

func matchResource(resources []string, resource string) bool {
	for _, r := range resources {
		if r == "*" || r == resource {
			return true
		}
	}
	return false
}

func matchVerb(verbs []string, verb string) bool {
	for _, v := range verbs {
		if v == "*" || v == verb {
			return true
		}
	}
	return false
}

func (e *Engine) ResolveFleetGroups(ctx context.Context, ldapGroups []string) []string {
	var groupList authv1.FleetGroupList
	if err := e.Client.List(ctx, &groupList, client.InNamespace(e.Namespace)); err != nil {
		return ldapGroups
	}
	merged := map[string]struct{}{}
	for _, g := range ldapGroups {
		merged[g] = struct{}{}
	}
	for _, fg := range groupList.Items {
		if fg.Spec.LDAPGroupCN != "" {
			for _, lg := range ldapGroups {
				if lg == fg.Spec.LDAPGroupCN {
					merged[fg.Name] = struct{}{}
				}
			}
		}
	}
	out := make([]string, 0, len(merged))
	for g := range merged {
		out = append(out, g)
	}
	return out
}

func (e *Engine) LoadServiceAccount(ctx context.Context, name string) (*authv1.FleetServiceAccount, error) {
	var sa authv1.FleetServiceAccount
	err := e.Client.Get(ctx, types.NamespacedName{Namespace: e.Namespace, Name: name}, &sa)
	if err != nil {
		return nil, err
	}
	return &sa, nil
}
