package reconcile

import (
	"context"
	"fmt"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/dns"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *RouteReconciler) discoverPending(ctx context.Context, prevRoutesYAML, routesYAML []byte) []string {
	var list fleetv1alpha1.FleetAppRouteList
	if err := r.List(ctx, &list); err != nil {
		return discoverPendingHostnames(ctx, r.Config.GatewayProbeHost, prevRoutesYAML, routesYAML)
	}
	probeHost := effectiveGatewayProbeHost(r.Config)
	seen := map[string]struct{}{}
	var pending []string
	add := func(host string, skipPermanent bool) {
		host = normalizeHostname(host)
		if host == "" {
			return
		}
		if _, dup := seen[host]; dup {
			return
		}
		seen[host] = struct{}{}
		if skipPermanent {
			for _, route := range list.Items {
				if normalizeHostname(route.Spec.HubHost) == host {
					if route.Status.TLSPhase == fleetv1alpha1.TLSPhaseFailedDNS ||
						route.Status.TLSPhase == fleetv1alpha1.TLSPhaseFailedRateLimit {
						return
					}
				}
			}
		}
		if hostnameNeedsProduction(ctx, probeHost, host) {
			pending = append(pending, host)
		}
	}
	for _, route := range list.Items {
		mode := route.Spec.EffectiveMode
		if mode == "" {
			mode = route.Spec.Mode
		}
		if mode != fleetv1alpha1.RouteModeHub && mode != fleetv1alpha1.RouteModeDual {
			continue
		}
		if route.Status.TLSPhase == fleetv1alpha1.TLSPhaseReady {
			continue
		}
		add(route.Spec.HubHost, true)
	}
	if r.Config.AutheliaEnabled {
		portal := r.autheliaPortalHost()
		if hostnameNeedsProduction(ctx, probeHost, portal) {
			add(portal, false)
		}
	}
	return pending
}

func (r *RouteReconciler) validateDNS(ctx context.Context, hostname string) (bool, string) {
	expected := effectiveGatewayWANIP(r.Config)
	if expected == "" {
		expected = effectiveGatewayProbeHost(r.Config)
	}
	ok, ips, err := dns.ValidateHost(ctx, hostname, expected)
	if err != nil {
		return false, err.Error()
	}
	if !ok {
		return false, fmt.Sprintf("DNS %v does not match expected %s", ips, expected)
	}
	return true, ""
}

func (r *RouteReconciler) updateTLSPhase(ctx context.Context, hostname, phase, msg string) error {
	hostname = normalizeHostname(hostname)
	var list fleetv1alpha1.FleetAppRouteList
	if err := r.List(ctx, &list); err != nil {
		return err
	}
	var last error
	for _, route := range list.Items {
		if normalizeHostname(route.Spec.HubHost) != hostname {
			continue
		}
		if err := r.patchTLSStatus(ctx, &route, phase, msg); err != nil {
			last = err
		}
	}
	if last != nil {
		return last
	}
	if r.Config.AutheliaEnabled && normalizeHostname(r.autheliaPortalHost()) == hostname {
		return nil
	}
	return nil
}

func (r *RouteReconciler) onCertReady(ctx context.Context, hostname string) error {
	if err := r.updateTLSPhase(ctx, hostname, fleetv1alpha1.TLSPhaseReady, ""); err != nil {
		return err
	}
	// Invalidate rebuild keys so the next route reconcile refreshes fleet-routes once.
	var list fleetv1alpha1.FleetAppRouteList
	if err := r.List(ctx, &list); err != nil {
		return err
	}
	hostname = normalizeHostname(hostname)
	for _, route := range list.Items {
		if normalizeHostname(route.Spec.HubHost) != hostname {
			continue
		}
		_ = r.clearFleetRoutesRebuildKey(ctx, &route)
	}
	if r.Config.AutheliaEnabled && normalizeHostname(r.autheliaPortalHost()) == hostname {
		// Auth portal is not a FleetAppRoute; one global rebuild is enough.
		_, err := r.rebuildFleetRoutesCM(ctx)
		return err
	}
	return nil
}

func (r *RouteReconciler) clearFleetRoutesRebuildKey(ctx context.Context, route *fleetv1alpha1.FleetAppRoute) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest fleetv1alpha1.FleetAppRoute
		if err := r.Get(ctx, types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, &latest); err != nil {
			return err
		}
		if latest.Annotations == nil || latest.Annotations[annotationFleetRoutesRebuildKey] == "" {
			return nil
		}
		base := latest.DeepCopy()
		delete(latest.Annotations, annotationFleetRoutesRebuildKey)
		return r.Patch(ctx, &latest, client.MergeFrom(base))
	})
}

func (r *RouteReconciler) patchTLSStatus(ctx context.Context, route *fleetv1alpha1.FleetAppRoute, phase, msg string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest fleetv1alpha1.FleetAppRoute
		if err := r.Get(ctx, client.ObjectKeyFromObject(route), &latest); err != nil {
			return err
		}
		base := latest.DeepCopy()
		latest.Status.TLSPhase = phase
		latest.Status.TLSMessage = msg
		r.setCertificateReadyCondition(&latest, phase == fleetv1alpha1.TLSPhaseReady, msg)
		return r.Status().Patch(ctx, &latest, client.MergeFrom(base))
	})
}

func (r *RouteReconciler) setCertificateReadyCondition(route *fleetv1alpha1.FleetAppRoute, ready bool, msg string) {
	status := metav1.ConditionFalse
	reason := "Pending"
	if ready {
		status = metav1.ConditionTrue
		reason = "Ready"
	}
	now := metav1.Now()
	cond := metav1.Condition{
		Type:               fleetv1alpha1.ConditionCertificateReady,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		LastTransitionTime: now,
	}
	found := false
	for i, c := range route.Status.Conditions {
		if c.Type == fleetv1alpha1.ConditionCertificateReady {
			if c.Status == status {
				cond.LastTransitionTime = c.LastTransitionTime
			}
			route.Status.Conditions[i] = cond
			found = true
			break
		}
	}
	if !found {
		route.Status.Conditions = append(route.Status.Conditions, cond)
	}
}

func (r *RouteReconciler) buildCertsReadyMap(ctx context.Context, routes []fleetv1alpha1.FleetAppRoute) map[string]bool {
	if !r.Config.ACMEEnabled {
		return nil
	}
	probeHost := effectiveGatewayProbeHost(r.Config)
	out := map[string]bool{}
	add := func(host string) {
		host = normalizeHostname(host)
		if host == "" {
			return
		}
		if _, ok := out[host]; ok {
			return
		}
		ok, err := probeACMECert(probeHost, host, false)
		out[host] = err == nil && ok
	}
	for _, route := range routes {
		mode := route.Spec.EffectiveMode
		if mode == "" {
			mode = route.Spec.Mode
		}
		if mode == fleetv1alpha1.RouteModeHub || mode == fleetv1alpha1.RouteModeDual {
			add(route.Spec.HubHost)
		}
	}
	if r.Config.AutheliaEnabled {
		add(r.autheliaPortalHost())
	}
	return out
}
