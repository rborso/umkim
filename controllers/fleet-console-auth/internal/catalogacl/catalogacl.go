package catalogacl

import (
	"context"
	"encoding/json"
	"strings"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/rbac"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Manager struct {
	Client    client.Client
	Namespace string
	RBAC      *rbac.Engine
}

func (m *Manager) CanRead(ctx context.Context, actor rbac.Actor, catalogItemID string) (bool, error) {
	acl, err := m.lookup(ctx, catalogItemID)
	if err != nil {
		return false, err
	}
	if acl == nil || strings.EqualFold(acl.Spec.Visibility, "public") {
		return true, nil
	}
	if ok, _ := m.RBAC.IsDCAdmin(ctx, actor); ok {
		return true, nil
	}
	for _, g := range acl.Spec.Grants {
		if grantMatches(actor, g) && (g.AccessLevel == "read" || g.AccessLevel == "readWrite") {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) CanWrite(ctx context.Context, actor rbac.Actor, catalogItemID string) (bool, error) {
	acl, err := m.lookup(ctx, catalogItemID)
	if err != nil {
		return false, err
	}
	if ok, _ := m.RBAC.IsDCAdmin(ctx, actor); ok {
		return true, nil
	}
	if acl == nil {
		return okRBAC(ctx, m.RBAC, actor, "catalog-items", "update"), nil
	}
	for _, g := range acl.Spec.Grants {
		if grantMatches(actor, g) && g.AccessLevel == "readWrite" {
			return true, nil
		}
	}
	return okRBAC(ctx, m.RBAC, actor, "catalog-items", "update"), nil
}

func (m *Manager) FilterList(ctx context.Context, actor rbac.Actor, body []byte) ([]byte, error) {
	if ok, _ := m.RBAC.IsDCAdmin(ctx, actor); ok {
		return body, nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, nil
	}
	results, ok := payload["results"].([]interface{})
	if !ok {
		return body, nil
	}
	var filtered []interface{}
	for _, item := range results {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id := catalogID(obj)
		if id == "" {
			filtered = append(filtered, item)
			continue
		}
		allowed, err := m.CanRead(ctx, actor, id)
		if err == nil && allowed {
			filtered = append(filtered, item)
		}
	}
	payload["results"] = filtered
	return json.Marshal(payload)
}

func (m *Manager) lookup(ctx context.Context, catalogItemID string) (*authv1.FleetCatalogAccess, error) {
	var list authv1.FleetCatalogAccessList
	if err := m.Client.List(ctx, &list, client.InNamespace(m.Namespace)); err != nil {
		return nil, err
	}
	for i := range list.Items {
		if list.Items[i].Spec.CatalogItemID == catalogItemID {
			return &list.Items[i], nil
		}
	}
	return nil, nil
}

func grantMatches(actor rbac.Actor, g authv1.CatalogAccessGrant) bool {
	switch g.Subject.Kind {
	case "User":
		return actor.Kind == "User" && actor.Name == g.Subject.Name
	case "Group":
		for _, grp := range actor.Groups {
			if grp == g.Subject.Name {
				return true
			}
		}
	case "ServiceAccount":
		return actor.Kind == "ServiceAccount" && actor.Name == g.Subject.Name
	}
	return false
}

func catalogID(obj map[string]interface{}) string {
	for _, k := range []string{"uid", "id", "name"} {
		if v, ok := obj[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func okRBAC(ctx context.Context, e *rbac.Engine, actor rbac.Actor, resource, verb string) bool {
	ok, _ := e.Authorize(ctx, actor, rbac.Request{Resource: resource, Verb: verb})
	return ok
}
