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
	_, ci, ok := r.clusterEdgeSiteAndIngress(ctx, clusterName)
	if !ok || ci.Method != fleetv1alpha1.IngressMethodPortForward {
		return ""
	}
	return ci.Zone
}

// clusterEdgeSiteAndIngress loads the cluster's site and its resolved ingress.
// The site is nil only when the cluster or site cannot be read; ciOK is false
// when the cluster has no resolvable ingress binding.
func (r *AppReconciler) clusterEdgeSiteAndIngress(ctx context.Context, clusterName string) (*fleetv1alpha1.FleetSite, ingress.ClusterIngress, bool) {
	if clusterName == "" {
		return nil, ingress.ClusterIngress{}, false
	}
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: clusterName}, &fcj); err != nil {
		return nil, ingress.ClusterIngress{}, false
	}
	siteID := strings.TrimSpace(fcj.Spec.SiteId)
	if siteID == "" {
		return nil, ingress.ClusterIngress{}, false
	}
	var fs fleetv1alpha1.FleetSite
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: siteID}, &fs); err != nil {
		return nil, ingress.ClusterIngress{}, false
	}
	ingressID := strings.TrimSpace(fcj.Spec.Edge.IngressId)
	if ingressID == "" {
		ingressID = fs.Spec.AutoIngressID()
	}
	if ingressID == "" {
		return &fs, ingress.ClusterIngress{}, false
	}
	ci, err := ingress.ResolveClusterIngress(fs.Spec, ingressID)
	if err != nil {
		return &fs, ingress.ClusterIngress{}, false
	}
	return &fs, ci, true
}
