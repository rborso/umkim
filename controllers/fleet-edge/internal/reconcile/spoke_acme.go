package reconcile

import (
	"context"
	"fmt"
	"strings"
	"sync"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/dns"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// spokeACMEQueue serializes TLS ingress pushes per cluster (one site hostname at a time).
type spokeACMEQueue struct {
	mu      sync.Mutex
	active  map[string]string
	pending map[string][]string
}

func newSpokeACMEQueue() *spokeACMEQueue {
	return &spokeACMEQueue{
		active:  map[string]string{},
		pending: map[string][]string{},
	}
}

func (q *spokeACMEQueue) allow(cluster, host string) bool {
	host = normalizeHostname(host)
	cluster = normalizeHostname(cluster)
	if cluster == "" || host == "" {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if cur, ok := q.active[cluster]; ok {
		if cur == host {
			return true
		}
		if !q.pendingContainsLocked(cluster, host) {
			q.pending[cluster] = append(q.pending[cluster], host)
		}
		return false
	}
	q.active[cluster] = host
	return true
}

func (q *spokeACMEQueue) pendingContainsLocked(cluster, host string) bool {
	for _, h := range q.pending[cluster] {
		if h == host {
			return true
		}
	}
	return false
}

func (q *spokeACMEQueue) release(cluster, host string) {
	host = normalizeHostname(host)
	cluster = normalizeHostname(cluster)
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.active[cluster] != host {
		return
	}
	delete(q.active, cluster)
	if len(q.pending[cluster]) > 0 {
		next := q.pending[cluster][0]
		q.pending[cluster] = q.pending[cluster][1:]
		q.active[cluster] = next
	}
}

func (q *spokeACMEQueue) activeHost(cluster string) string {
	cluster = normalizeHostname(cluster)
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.active[cluster]
}

func (r *RouteReconciler) advanceSpokeACMEQueue(ctx context.Context, clusterName string, regCluster clusters.Cluster) {
	if r.spokeACME == nil || !regCluster.SpokeACMEEnabled(r.Config.ACMEEnabled) {
		return
	}
	probeHost := ""
	for i := 0; i < 8; i++ {
		host := r.spokeACME.activeHost(clusterName)
		if host == "" {
			return
		}
		if probeHost == "" {
			probeHost = spokeProbeHost(ctx, host, regCluster)
		}
		route := r.routeForSiteHost(ctx, host, clusterName)
		if route == nil {
			return
		}

		if r.Config.SpokeACMEPreflightEnabled && !spokeStagingOK(route) {
			ok, err := probeACMECert(probeHost, host, true)
			if err != nil || !ok {
				return
			}
			_ = r.annotateSpokeStagingOK(ctx, route)
			_ = r.patchTLSStatus(ctx, route, fleetv1alpha1.TLSPhaseIssuing, "staging preflight succeeded; requesting production certificate")
			return
		}

		ok, err := probeACMECert(probeHost, host, false)
		if err != nil || !ok {
			return
		}
		r.spokeACME.release(clusterName, host)
		_ = r.markSiteHostTLSReady(ctx, clusterName, host)
	}
}

func spokeStagingOK(route *fleetv1alpha1.FleetAppRoute) bool {
	if route == nil || route.Annotations == nil {
		return false
	}
	return route.Annotations[annotationSpokeACMEStagingOK] == "true"
}

func (r *RouteReconciler) routeForSiteHost(ctx context.Context, siteHost, clusterName string) *fleetv1alpha1.FleetAppRoute {
	siteHost = normalizeHostname(siteHost)
	var list fleetv1alpha1.FleetAppRouteList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}
	for i := range list.Items {
		route := &list.Items[i]
		if normalizeHostname(route.Spec.SiteHost) != siteHost {
			continue
		}
		if routeTargetsCluster(route, clusterName) {
			return route
		}
	}
	return nil
}

func (r *RouteReconciler) annotateSpokeStagingOK(ctx context.Context, route *fleetv1alpha1.FleetAppRoute) error {
	if route == nil {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest fleetv1alpha1.FleetAppRoute
		if err := r.Get(ctx, types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, &latest); err != nil {
			return err
		}
		if latest.Annotations != nil && latest.Annotations[annotationSpokeACMEStagingOK] == "true" {
			return nil
		}
		base := latest.DeepCopy()
		if latest.Annotations == nil {
			latest.Annotations = map[string]string{}
		}
		latest.Annotations[annotationSpokeACMEStagingOK] = "true"
		return r.Patch(ctx, &latest, client.MergeFrom(base))
	})
}

func (r *RouteReconciler) markSiteHostTLSReady(ctx context.Context, clusterName, siteHost string) error {
	siteHost = normalizeHostname(siteHost)
	clusterName = normalizeHostname(clusterName)
	var list fleetv1alpha1.FleetAppRouteList
	if err := r.List(ctx, &list); err != nil {
		return err
	}
	var last error
	for i := range list.Items {
		route := &list.Items[i]
		if normalizeHostname(route.Spec.SiteHost) != siteHost {
			continue
		}
		if route.Spec.Cluster != clusterName && !routeTargetsCluster(route, clusterName) {
			continue
		}
		if err := r.patchTLSStatus(ctx, route, fleetv1alpha1.TLSPhaseReady, ""); err != nil {
			last = err
		}
	}
	return last
}

func routeTargetsCluster(route *fleetv1alpha1.FleetAppRoute, clusterName string) bool {
	if route.Spec.Cluster == clusterName {
		return true
	}
	for _, t := range route.Spec.ClusterTargets {
		if t.Cluster == clusterName {
			return true
		}
	}
	return false
}

func (r *RouteReconciler) validateSiteDNS(ctx context.Context, hostname string, cluster clusters.Cluster) (bool, string) {
	expected := strings.TrimSpace(cluster.EdgeWANIP)
	relaxed := cluster.DNSValidationRelaxed() ||
		(cluster.EffectiveMethod() == "port_forward" && isPrivateProbeHost(expected))
	if relaxed {
		if _, ips, err := dns.ValidateHost(ctx, hostname, ""); err != nil || len(ips) == 0 {
			return false, fmt.Sprintf("hostname %s did not resolve", hostname)
		}
		if ok, msg := validateSpokeConnectivityForCluster(ctx, hostname, cluster); !ok {
			return false, msg
		}
		return true, ""
	}
	ok, ips, err := dns.ValidateHost(ctx, hostname, expected)
	if err != nil {
		return false, err.Error()
	}
	if !ok {
		if expected == "" {
			return false, fmt.Sprintf("hostname %s did not resolve", hostname)
		}
		return false, fmt.Sprintf("DNS %v does not match expected %s", ips, expected)
	}
	return true, ""
}
