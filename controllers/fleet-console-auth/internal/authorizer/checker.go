package authorizer

import (
	"context"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/rbac"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	managedClusterGVK = schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	}
)

type Checker struct {
	RBAC   *rbac.Engine
	Client client.Client
}

func (c *Checker) Allow(ctx context.Context, actor rbac.Actor, mapped MappedRequest) (bool, error) {
	if mapped.Resource == "health" {
		return true, nil
	}
	req := rbac.Request{
		Resource:    mapped.Resource,
		Verb:        mapped.Verb,
		ScopeKind:   mapped.ScopeKind,
		ScopeName:   mapped.ScopeName,
		AccessLevel: mapped.AccessLevel,
	}
	ok, err := c.RBAC.Authorize(ctx, actor, req)
	if err != nil || ok {
		return ok, err
	}
	if mapped.ScopeKind == "Cluster" && mapped.ScopeName != "" {
		siteID, _ := c.clusterSite(ctx, mapped.ScopeName)
		if siteID != "" {
			req.ScopeKind = "Site"
			req.ScopeName = siteID
			return c.RBAC.Authorize(ctx, actor, req)
		}
	}
	return false, nil
}

func (c *Checker) clusterSite(ctx context.Context, clusterName string) (string, error) {
	labels, err := c.managedClusterLabels(ctx, clusterName)
	if err != nil {
		return "", err
	}
	if labels == nil {
		return "", nil
	}
	return labels["fleet.site"], nil
}

func (c *Checker) managedClusterLabels(ctx context.Context, clusterName string) (map[string]string, error) {
	var mc unstructured.Unstructured
	mc.SetGroupVersionKind(managedClusterGVK)
	if err := c.Client.Get(ctx, client.ObjectKey{Name: clusterName}, &mc); err != nil {
		return nil, err
	}
	return mc.GetLabels(), nil
}

func ActorFromClaims(kind, name string, groups []string) rbac.Actor {
	return rbac.Actor{Kind: kind, Name: name, Groups: groups}
}

func SubjectRefFromActor(actor rbac.Actor) authv1.SubjectRef {
	return authv1.SubjectRef{Kind: actor.Kind, Name: actor.Name}
}

func roleNames(ctx context.Context, eng *rbac.Engine, actor rbac.Actor) []string {
	if eng == nil {
		return nil
	}
	_, bindings, err := eng.LoadPolicy(ctx)
	if err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	for _, b := range bindings {
		match := false
		for _, sub := range b.Spec.Subjects {
			switch sub.Kind {
			case "User":
				if actor.Kind == "User" && actor.Name == sub.Name {
					match = true
				}
			case "Group":
				for _, g := range actor.Groups {
					if g == sub.Name {
						match = true
						break
					}
				}
			case "ServiceAccount":
				if actor.Kind == "ServiceAccount" && actor.Name == sub.Name {
					match = true
				}
			}
			if match {
				break
			}
		}
		if match && b.Spec.RoleRef.Name != "" {
			seen[b.Spec.RoleRef.Name] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	return out
}
