package reconcile

import (
	"context"
	"fmt"
	"net/http"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/config"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/health"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RouteHealthReconciler probes hub-routed FleetAppRoutes and updates health status.
type RouteHealthReconciler struct {
	client.Client
	Config     config.Config
	HTTPClient *http.Client
}

func (r *RouteHealthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var route fleetv1alpha1.FleetAppRoute
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if route.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	mode := route.Spec.EffectiveMode
	if mode == "" {
		mode = route.Spec.Mode
	}
	if mode != fleetv1alpha1.RouteModeHub {
		return ctrl.Result{RequeueAfter: time.Duration(r.Config.RouteHealthIntervalSec) * time.Second}, nil
	}
	if route.Status.Phase == "" || route.Status.Phase == fleetv1alpha1.PhasePending {
		return ctrl.Result{RequeueAfter: time.Duration(r.Config.RouteHealthIntervalSec) * time.Second}, nil
	}

	gatewayURL := health.HubGatewayProbeURL(r.Config.HubGatewayURL, r.Config.HubGatewayHTTPURL, route.Status.TLSPhase)
	healthy, code, err := health.ProbeHubPath(
		ctx, r.HTTPClient, gatewayURL, route.Spec.HubHost, route.EffectiveHealthPath(),
	)
	now := metav1.Now()
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest fleetv1alpha1.FleetAppRoute
		if err := r.Get(ctx, req.NamespacedName, &latest); err != nil {
			return err
		}
		base := latest.DeepCopy()

		if latest.Status.ActiveCluster == "" {
			latest.Status.ActiveCluster = latest.Spec.Cluster
		}
		if latest.Status.FailoverPhase == "" {
			latest.Status.FailoverPhase = fleetv1alpha1.FailoverPhaseIdle
		}

		if err != nil {
			latest.Status.HubPathHealthy = false
			latest.Status.ConsecutiveSuccess = 0
			latest.Status.ConsecutiveFailures++
			latest.Status.LastProbeAt = &now
			setHubRouteCondition(&latest, false, err.Error())
		} else if healthy {
			latest.Status.HubPathHealthy = true
			latest.Status.ConsecutiveFailures = 0
			latest.Status.ConsecutiveSuccess++
			latest.Status.LastProbeAt = &now
			setHubRouteCondition(&latest, true, fmt.Sprintf("HTTP %d", code))
			if latest.Status.ConsecutiveSuccess >= r.Config.HealthSuccessThreshold {
				if latest.Status.FailoverPhase == fleetv1alpha1.FailoverPhaseDegraded {
					latest.Status.FailoverPhase = fleetv1alpha1.FailoverPhaseIdle
				}
				if latest.Status.Phase == fleetv1alpha1.PhaseDegraded && latest.Status.Message == "" {
					latest.Status.Phase = fleetv1alpha1.PhaseApplied
				}
			}
		} else {
			latest.Status.HubPathHealthy = false
			latest.Status.ConsecutiveSuccess = 0
			latest.Status.ConsecutiveFailures++
			latest.Status.LastProbeAt = &now
			setHubRouteCondition(&latest, false, fmt.Sprintf("HTTP %d", code))
		}

		if latest.Status.ConsecutiveFailures >= r.Config.HealthFailureThreshold {
			latest.Status.FailoverPhase = fleetv1alpha1.FailoverPhaseDegraded
			if latest.Status.Phase == fleetv1alpha1.PhaseApplied {
				latest.Status.Phase = fleetv1alpha1.PhaseDegraded
				latest.Status.Message = fmt.Sprintf("hub path unhealthy after %d probes", latest.Status.ConsecutiveFailures)
			}
			// Log once per threshold crossing, not every probe tick.
			if latest.Status.ConsecutiveFailures == r.Config.HealthFailureThreshold {
				logger.Info("hub route degraded", "host", latest.Spec.HubHost, "failures", latest.Status.ConsecutiveFailures)
			}
		}
		return r.Status().Patch(ctx, &latest, client.MergeFrom(base))
	}); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Duration(r.Config.RouteHealthIntervalSec) * time.Second}, nil
}

func setHubRouteCondition(route *fleetv1alpha1.FleetAppRoute, healthy bool, msg string) {
	status := metav1.ConditionFalse
	if healthy {
		status = metav1.ConditionTrue
	}
	route.Status.Conditions = mergeCondition(route.Status.Conditions, metav1.Condition{
		Type:               fleetv1alpha1.ConditionHubRouteReachable,
		Status:             status,
		Reason:             "Probe",
		Message:            msg,
		LastTransitionTime: metav1.Now(),
	})
}

func mergeCondition(conds []metav1.Condition, c metav1.Condition) []metav1.Condition {
	out := make([]metav1.Condition, 0, len(conds)+1)
	found := false
	for _, existing := range conds {
		if existing.Type == c.Type {
			if existing.Status == c.Status && existing.Message == c.Message {
				c.LastTransitionTime = existing.LastTransitionTime
			}
			out = append(out, c)
			found = true
		} else {
			out = append(out, existing)
		}
	}
	if !found {
		out = append(out, c)
	}
	return out
}

func (r *RouteHealthReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.HTTPClient == nil {
		r.HTTPClient = health.NewHTTPClient(15 * time.Second)
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("fleetapproute-health").
		For(&fleetv1alpha1.FleetAppRoute{}).
		Complete(r)
}
