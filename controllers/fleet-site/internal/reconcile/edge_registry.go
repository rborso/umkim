package reconcile

import (
	"context"

	"github.com/rborso/umkim/controllers/fleet-site/internal/registry"
)

func (r *Reconciler) reconcileEdgeDomains(ctx context.Context) error {
	return registry.ReconcileEdgeDomains(ctx, r.Client, registry.DomainConfig{
		FleetSiteNamespace:      r.Cfg.Namespace,
		EdgeDomainsCMNamespace:  r.Cfg.EdgeDomainsCMNamespace,
		EdgeDomainsCMName:       r.Cfg.EdgeDomainsCMName,
		EdgeClustersCMNamespace: r.Cfg.EdgeClustersCMNamespace,
		EdgeClustersCMName:      r.Cfg.EdgeClustersCMName,
		HubEdgeDomain:           r.Cfg.HubEdgeDomain,
		ManagementSiteID:        r.Cfg.ManagementSiteID,
	})
}
