package reconcile

import (
	"context"
	"fmt"
	"strings"
	"sync"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/dns"
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
	probeHost := strings.TrimSpace(regCluster.EdgeWANIP)
	if probeHost == "" {
		probeHost = effectiveGatewayProbeHost(r.Config)
	}
	for i := 0; i < 8; i++ {
		host := r.spokeACME.activeHost(clusterName)
		if host == "" {
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
	expected := cluster.EdgeWANIP
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

func (r *RouteReconciler) spokeACMEEnabledForRoute(ctx context.Context, route *fleetv1alpha1.FleetAppRoute, clusterName string, regCluster clusters.Cluster) (bool, string) {
	if !regCluster.SpokeACMEEnabled(r.Config.ACMEEnabled) {
		return false, ""
	}
	mode := route.Spec.EffectiveMode
	if mode == "" {
		mode = route.Spec.Mode
	}
	if mode != fleetv1alpha1.RouteModeSite && mode != fleetv1alpha1.RouteModeDual {
		return false, ""
	}
	siteHost := route.Spec.SiteHost
	if siteHost == "" {
		return false, ""
	}
	if route.Status.TLSPhase == fleetv1alpha1.TLSPhaseFailedDNS {
		return false, route.Status.TLSMessage
	}
	if r.Config.ACMEDNSCheckEnabled {
		ok, msg := r.validateSiteDNS(ctx, siteHost, regCluster)
		if !ok {
			_ = r.patchTLSStatus(ctx, route, fleetv1alpha1.TLSPhaseFailedDNS, msg)
			return false, msg
		}
	}
	if r.spokeACME == nil {
		r.spokeACME = newSpokeACMEQueue()
	}
	if !r.spokeACME.allow(clusterName, siteHost) {
		_ = r.patchTLSStatus(ctx, route, fleetv1alpha1.TLSPhaseQueued, "waiting for spoke ACME queue")
		// Still "enabled" for rendering: do not downgrade an existing TLS ManifestWork to
		// plain web-hub while waiting — caller skips ApplyYAML until allow() succeeds.
		return true, "queued"
	}
	return true, ""
}
