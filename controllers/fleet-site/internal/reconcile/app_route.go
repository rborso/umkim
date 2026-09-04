package reconcile

import (
	"context"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingress"
	"k8s.io/apimachinery/pkg/types"
)

func defaultRouteHostname(release, domain string) string {
	release = strings.TrimSpace(release)
	domain = strings.Trim(strings.TrimSpace(domain), ".")
	if release == "" || domain == "" {
		return ""
	}
	return release + "." + domain
}

func (r *AppReconciler) clusterEdgeDomain(ctx context.Context, clusterName string) string {
	if clusterName == "" {
		return ""
	}
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: clusterName}, &fcj); err != nil {
		return ""
	}
	siteID := strings.TrimSpace(fcj.Spec.SiteId)
	if siteID == "" {
		return ""
	}
	var fs fleetv1alpha1.FleetSite
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: siteID}, &fs); err != nil {
		return ""
	}
	ingressID := strings.TrimSpace(fcj.Spec.Edge.IngressId)
	if ingressID == "" {
		ingressID = fs.Spec.AutoIngressID()
	}
	if ingressID == "" {
		return ""
	}
	ci, err := ingress.ResolveClusterIngress(fs.Spec, ingressID)
	if err != nil || ci.Method != fleetv1alpha1.IngressMethodPortForward {
		return ""
	}
	return ci.Zone
}
