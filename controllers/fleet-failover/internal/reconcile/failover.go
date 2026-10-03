package reconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	fleetedge "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-failover/internal/config"
	"github.com/rborso/umkim/controllers/fleet-failover/internal/placement"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	labelHubRouteDegraded     = "fleet.health/hub_route"
	annotationDegradedUntil   = "fleet.umkim.com/degraded-until"
	annotationRelocateGen     = "fleet.umkim.com/relocate-generation"
	annotationLastFailoverAt  = "fleet.umkim.com/last-failover-at"
	annotationFailoverGen     = "fleet.umkim.com/failover-generation"
	annotationResourceID      = "fleet.umkim.com/resource-id"
)

var managedClusterGVK = schema.GroupVersionKind{
	Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
}

type FailoverReconciler struct {
	client.Client
	Config     config.Config
	Placement  placement.Client
}

func (r *FailoverReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var route fleetedge.FleetAppRoute
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if route.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	active := route.ActiveClusterName()
	_ = active // cluster identity for failover actuation; placement exclusion uses degraded CM only
	degraded := route.Status.FailoverPhase == fleetedge.FailoverPhaseDegraded ||
		(route.Status.ConsecutiveFailures >= 3 && !route.Status.HubPathHealthy)

	if degraded {
		// Degraded CM is for console/failover actuation only. Do not label ManagedCluster
		// fleet.health/hub_route=degraded — policy-manager excludes labeled clusters from
		// all new placements, which blocks unrelated deploys when hub routes are unhealthy.
		if err := r.syncDegradedCM(ctx); err != nil {
			logger.Error(err, "sync degraded CM")
		}
		return r.actOnDegraded(ctx, &route)
	}

	// Clear legacy degrade labels left by older controllers when the route recovers.
	if err := r.clearClusterDegraded(ctx, active); err != nil {
		logger.Error(err, "clear cluster degrade", "cluster", active)
	}
	_ = r.syncDegradedCM(ctx)
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *FailoverReconciler) actOnDegraded(ctx context.Context, route *fleetedge.FleetAppRoute) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	mode := route.Spec.FailoverMode
	if mode == "" || mode == fleetedge.FailoverModeManual {
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}
	if withinCooldown(route, r.Config.FailoverCooldownSeconds) {
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	switch mode {
	case fleetedge.FailoverModeStandby:
		if route.Spec.StandbyCluster == "" {
			return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
		}
		if route.Status.ActiveCluster == route.Spec.StandbyCluster {
			return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
		}
		now := time.Now().UTC().Format(time.RFC3339)
		base := route.DeepCopy()
		if route.Annotations == nil {
			route.Annotations = map[string]string{}
		}
		route.Annotations[annotationLastFailoverAt] = now
		route.Annotations[annotationFailoverGen] = now
		if err := r.Patch(ctx, route, client.MergeFrom(base)); err != nil {
			return ctrl.Result{}, err
		}
		statusBase := route.DeepCopy()
		route.Status.ActiveCluster = route.Spec.StandbyCluster
		route.Status.FailoverPhase = fleetedge.FailoverPhaseIdle
		route.Status.ConsecutiveFailures = 0
		if err := r.Status().Patch(ctx, route, client.MergeFrom(statusBase)); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("standby failover", "instance", route.Spec.InstanceID, "cluster", route.Spec.StandbyCluster)
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil

	case fleetedge.FailoverModeRelocatable:
		if route.Status.FailoverPhase == fleetedge.FailoverPhaseRelocating {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		resourceID := route.Annotations[annotationResourceID]
		if resourceID == "" {
			return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
		}
		patch := client.MergeFrom(route.DeepCopy())
		route.Status.FailoverPhase = fleetedge.FailoverPhaseRelocating
		_ = r.Status().Patch(ctx, route, patch)

		resp, err := r.Placement.Relocate(ctx, resourceID, placement.RelocateRequest{
			ExcludeClusters: []string{route.ActiveClusterName()},
			Reason:          "hub_path_degraded",
		})
		if err != nil {
			failPatch := client.MergeFrom(route.DeepCopy())
			route.Status.FailoverPhase = fleetedge.FailoverPhaseFailed
			route.Status.Message = err.Error()
			_ = r.Status().Patch(ctx, route, failPatch)
			return ctrl.Result{RequeueAfter: 120 * time.Second}, err
		}
		newCluster := resp.ActiveCluster
		if newCluster == "" {
			newCluster = providerToCluster(resp.Provider)
		}
		upPatch := client.MergeFrom(route.DeepCopy())
		route.Spec.Cluster = newCluster
		route.Status.ActiveCluster = newCluster
		route.Status.FailoverPhase = fleetedge.FailoverPhaseIdle
		route.Status.ConsecutiveFailures = 0
		route.Status.HubPathHealthy = false
		if route.Annotations == nil {
			route.Annotations = map[string]string{}
		}
		route.Annotations[annotationLastFailoverAt] = time.Now().UTC().Format(time.RFC3339)
		route.Annotations[annotationRelocateGen] = fmt.Sprintf("%d", time.Now().Unix())
		if err := r.Patch(ctx, route, upPatch); err != nil {
			return ctrl.Result{}, err
		}
		_ = r.Status().Patch(ctx, route, client.MergeFrom(route.DeepCopy()))
		logger.Info("relocated instance", "instance", route.Spec.InstanceID, "cluster", newCluster)
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func withinCooldown(route *fleetedge.FleetAppRoute, sec int) bool {
	if route.Annotations == nil {
		return false
	}
	last := route.Annotations[annotationLastFailoverAt]
	if last == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, last)
	if err != nil {
		return false
	}
	return time.Since(t) < time.Duration(sec)*time.Second
}

func providerToCluster(provider string) string {
	if provider == "" {
		return ""
	}
	// k8s-virt01 -> virt01
	const prefix = "k8s-"
	if len(provider) > len(prefix) && provider[:len(prefix)] == prefix {
		return provider[len(prefix):]
	}
	return provider
}

func (r *FailoverReconciler) markClusterDegraded(ctx context.Context, cluster string) error {
	if cluster == "" {
		return nil
	}
	mc := &unstructured.Unstructured{}
	mc.SetGroupVersionKind(managedClusterGVK)
	if err := r.Get(ctx, types.NamespacedName{Name: cluster}, mc); err != nil {
		return client.IgnoreNotFound(err)
	}
	labels := mc.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[labelHubRouteDegraded] = "degraded"
	mc.SetLabels(labels)
	ann := mc.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann[annotationDegradedUntil] = time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	mc.SetAnnotations(ann)
	return r.Update(ctx, mc)
}

func (r *FailoverReconciler) clearClusterDegraded(ctx context.Context, cluster string) error {
	if cluster == "" {
		return nil
	}
	mc := &unstructured.Unstructured{}
	mc.SetGroupVersionKind(managedClusterGVK)
	if err := r.Get(ctx, types.NamespacedName{Name: cluster}, mc); err != nil {
		return client.IgnoreNotFound(err)
	}
	labels := mc.GetLabels()
	if labels == nil {
		return nil
	}
	if labels[labelHubRouteDegraded] != "degraded" {
		return nil
	}
	delete(labels, labelHubRouteDegraded)
	mc.SetLabels(labels)
	ann := mc.GetAnnotations()
	if ann != nil {
		delete(ann, annotationDegradedUntil)
		mc.SetAnnotations(ann)
	}
	return r.Update(ctx, mc)
}

func (r *FailoverReconciler) syncDegradedCM(ctx context.Context) error {
	var list fleetedge.FleetAppRouteList
	if err := r.List(ctx, &list); err != nil {
		return err
	}
	degraded := map[string]string{}
	for _, route := range list.Items {
		if route.Status.FailoverPhase == fleetedge.FailoverPhaseDegraded || (!route.Status.HubPathHealthy && route.Status.ConsecutiveFailures >= 3) {
			c := route.ActiveClusterName()
			if c != "" {
				degraded[c] = route.Spec.InstanceID
			}
		}
	}
	raw, _ := json.MarshalIndent(map[string]interface{}{"clusters": degraded}, "", "  ")
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Config.DegradedCMName,
			Namespace: r.Config.DegradedCMNamespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data["degraded.json"] = string(raw)
		return nil
	})
	if err != nil && apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *FailoverReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fleetedge.FleetAppRoute{}).
		Complete(r)
}
