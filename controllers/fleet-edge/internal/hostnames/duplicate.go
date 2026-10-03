// Package hostnames checks FleetAppRoute host collisions.
package hostnames

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/manifests"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FindDuplicate returns an error if host is already used by another route (excluding excludeRoute).
func FindDuplicate(ctx context.Context, c client.Client, ns, host, excludeRoute string) error {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return nil
	}

	var list fleetv1alpha1.FleetAppRouteList
	if err := c.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return err
	}

	for _, route := range list.Items {
		if route.Name == excludeRoute {
			continue
		}
		for _, h := range manifests.HostsForRoute(route) {
			if strings.EqualFold(strings.TrimSpace(h), host) {
				return fmt.Errorf("hostname %q already used by route %s", host, route.Name)
			}
		}
		if route.Spec.HubHost != "" && strings.EqualFold(route.Spec.HubHost, host) && route.Name != excludeRoute {
			return fmt.Errorf("hostname %q already used by route %s (hub)", host, route.Name)
		}
		if route.Spec.SiteHost != "" && strings.EqualFold(route.Spec.SiteHost, host) && route.Name != excludeRoute {
			return fmt.Errorf("hostname %q already used by route %s (site)", host, route.Name)
		}
	}
	return nil
}
