package reconcile

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/dns"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// spokeACMEQueue serializes TLS ingress pushes per cluster (one site hostname at a time).
type spokeACMEQueue struct {
	mu          sync.Mutex
	active      map[string]string
	pending     map[string][]string
	activeSince map[string]time.Time
}

func newSpokeACMEQueue() *spokeACMEQueue {
	return &spokeACMEQueue{
		active:      map[string]string{},
		pending:     map[string][]string{},
		activeSince: map[string]time.Time{},
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
	q.activeSince[cluster] = time.Now()
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
	q.promoteNextLocked(cluster)
}

// promoteNextLocked drops the current active host and activates the next pending
// one (if any), resetting the staleness clock.
func (q *spokeACMEQueue) promoteNextLocked(cluster string) {
	delete(q.active, cluster)
	delete(q.activeSince, cluster)
	if len(q.pending[cluster]) > 0 {
		next := q.pending[cluster][0]
		q.pending[cluster] = q.pending[cluster][1:]
		q.active[cluster] = next
		q.activeSince[cluster] = time.Now()
	}
}

// rotate moves the active host to the back of the queue so a hostname that never
// completes issuance cannot starve the other site hostnames for a cluster.
func (q *spokeACMEQueue) rotate(cluster string) {
	cluster = normalizeHostname(cluster)
	q.mu.Lock()
	defer q.mu.Unlock()
	cur, ok := q.active[cluster]
	if !ok {
		return
	}
	delete(q.active, cluster)
	delete(q.activeSince, cluster)
	if len(q.pending[cluster]) > 0 {
		next := q.pending[cluster][0]
		q.pending[cluster] = q.pending[cluster][1:]
		q.active[cluster] = next
		q.activeSince[cluster] = time.Now()
		q.pending[cluster] = append(q.pending[cluster], cur)
	} else {
		// Nothing else queued: keep it active but reset the clock so it is not
		// rotated in a tight loop.
		q.active[cluster] = cur
		q.activeSince[cluster] = time.Now()
	}
}

// staleFor reports how long the active host for a cluster has held the slot.
func (q *spokeACMEQueue) staleFor(cluster string) time.Duration {
	cluster = normalizeHostname(cluster)
	q.mu.Lock()
	defer q.mu.Unlock()
	since, ok := q.activeSince[cluster]
	if !ok {
		return 0
	}
	return time.Since(since)
}

func (q *spokeACMEQueue) activeHost(cluster string) string {
	cluster = normalizeHostname(cluster)
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.active[cluster]
}

// spokeACMEHostStaleAfter bounds how long one site hostname may hold a cluster's
// ACME slot without its certificate becoming ready. After this the hostname is
// rotated to the back of the queue so it cannot starve the others.
const spokeACMEHostStaleAfter = 3 * time.Minute

const (
	// spokeACMEProdRetryRefreshAfter is how long a production issuance attempt
	// may go without a live certificate before the ingress reload token is
	// re-stamped, giving Traefik a fresh router revision to retry from.
	spokeACMEProdRetryRefreshAfter = 3 * time.Minute
	// spokeACMEProdRetryRestartsAfter is the number of refreshes after which the
	// spoke Traefik is restarted once for that host. A restart is the retry
	// Traefik honours when the router configuration itself has not changed.
	spokeACMEProdRetryRestartsAfter = 2
)

func (r *RouteReconciler) advanceSpokeACMEQueue(ctx context.Context, clusterName string, regCluster clusters.Cluster) {
	if r.spokeACME == nil || !regCluster.SpokeACMEEnabled(r.Config.ACMEEnabled) {
		return
	}
	logger := log.FromContext(ctx).WithName("spoke-acme-queue").WithValues("cluster", clusterName)
	// The probe must use the port the spoke Traefik actually serves HTTPS on
	// (30443 for port_forward spokes). Probing :443 here made every check fail,
	// so the queue could only ever advance by rotating a host on staleness.
	probePort := spokeProbeHTTPSPort(regCluster)
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
			// Hostname no longer has a route (app deleted or renamed): drop it so
			// it cannot hold the cluster's ACME slot forever, and try the next.
			logger.Info("dropping queued host with no route", "host", host)
			r.spokeACME.release(clusterName, host)
			probeHost = ""
			continue
		}
		// A host whose certificate is already live must not hold the slot at all:
		// release it so the hostnames still waiting can be issued.
		if route.Status.TLSPhase == fleetv1alpha1.TLSPhaseReady {
			logger.Info("releasing slot for an already-issued host", "host", host)
			r.spokeACME.release(clusterName, host)
			probeHost = ""
			continue
		}

		if r.Config.SpokeACMEPreflightEnabled && !spokeStagingOK(route) {
			ok, err := probeACMECertOnPort(probeHost, probePort, host, true)
			if err != nil || !ok {
				if r.spokeACME.staleFor(clusterName) >= spokeACMEHostStaleAfter {
					logger.Info("rotating stale host (staging preflight)", "host", host, "stale", r.spokeACME.staleFor(clusterName).String())
					r.spokeACME.rotate(clusterName)
					probeHost = ""
					continue
				}
				return
			}
			_ = r.annotateSpokeStagingOK(ctx, route)
			_ = r.patchTLSStatus(ctx, route, fleetv1alpha1.TLSPhaseIssuing, "staging preflight succeeded; requesting production certificate")
			return
		}

		ok, err := probeACMECertOnPort(probeHost, probePort, host, false)
		if err != nil || !ok {
			// A stuck hostname must not block the rest of the cluster's queue:
			// rotate it to the back after the staleness window.
			if r.spokeACME.staleFor(clusterName) >= spokeACMEHostStaleAfter {
				logger.Info("rotating stale host (production certificate not live)", "host", host, "stale", r.spokeACME.staleFor(clusterName).String())
				r.spokeACME.rotate(clusterName)
				probeHost = ""
				continue
			}
			return
		}
		logger.Info("releasing slot for an issued host", "host", host)
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
	// The certificate is live: end the issuance episode so a later re-issuance
	// starts with a fresh escalation budget.
	r.clearProdRetryAttempts(clusterName, siteHost)
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
