package reconcile

import (
	"context"
	"fmt"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	annotationCleanupApps = "fleet.umkim.com/cleanup-apps"

	// appsCleanupTimeout bounds how long cluster delete waits for the cluster's
	// Helm releases to uninstall before it gives up on them and finishes, so an
	// unreachable spoke cannot wedge the delete.
	appsCleanupTimeout = 15 * time.Minute
)

func clusterCleanupApps(fcj *fleetv1alpha1.FleetClusterJoin) bool {
	return fcj.Annotations != nil && fcj.Annotations[annotationCleanupApps] == "true"
}

// reconcileClusterDeleteApps uninstalls the cluster's applications as the first
// stage of cluster delete, while the spoke is still reachable. It deletes the
// FleetApp objects and lets their own finalizer do the work — the Helm uninstall
// on the spoke plus the route and OIDC client cleanup — so the delete reuses the
// app lifecycle instead of duplicating it.
//
// done=false means "requeue after wait".
func (r *ClusterJoinReconciler) reconcileClusterDeleteApps(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (done bool, wait time.Duration, err error) {
	if !clusterCleanupApps(fcj) {
		return true, 0, nil
	}
	st := fcj.Status.AppsCleanup
	if st != nil && (st.Phase == fleetv1alpha1.AppsCleanupPhaseDone || st.Phase == fleetv1alpha1.AppsCleanupPhaseFailed) {
		return true, 0, nil
	}
	waitAfter := time.Duration(r.Cfg.RequeueIntervalSec) * time.Second
	if waitAfter <= 0 {
		waitAfter = 15 * time.Second
	}
	save := func() error {
		fcj.Status.AppsCleanup = st
		return r.Status().Update(ctx, fcj)
	}

	if st == nil || st.Phase == "" {
		apps, err := r.clusterAppNames(ctx, clusterName)
		if err != nil {
			return false, 0, err
		}
		now := metav1.NewTime(time.Now().UTC())
		st = &fleetv1alpha1.ClusterAppsCleanupStatus{
			Phase:     fleetv1alpha1.AppsCleanupPhaseUninstalling,
			Apps:      apps,
			StartedAt: &now,
		}
		if len(apps) == 0 {
			st.Phase = fleetv1alpha1.AppsCleanupPhaseDone
			st.Message = "no applications on this cluster"
			return true, 0, save()
		}
	}

	completed := make([]string, 0, len(st.Apps))
	remaining := make([]string, 0, len(st.Apps))
	for _, name := range st.Apps {
		var app fleetv1alpha1.FleetApp
		err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: name}, &app)
		if apierrors.IsNotFound(err) {
			completed = append(completed, name)
			continue
		}
		if err != nil {
			return false, 0, err
		}
		remaining = append(remaining, name)
		if app.DeletionTimestamp == nil {
			// The app's finalizer runs the Helm uninstall, removes the route and
			// the OIDC client, and only then drops the object.
			if err := r.Delete(ctx, &app); err != nil && !apierrors.IsNotFound(err) {
				return false, 0, err
			}
		}
	}
	st.Completed = completed

	if len(remaining) == 0 {
		st.Phase = fleetv1alpha1.AppsCleanupPhaseDone
		st.Message = fmt.Sprintf("%d application(s) uninstalled", len(completed))
		return true, 0, save()
	}

	started := time.Now()
	if st.StartedAt != nil {
		started = st.StartedAt.Time
	}
	if time.Since(started) > appsCleanupTimeout {
		// Leave the app objects alone: a finalizer is only removed once its
		// cleanup ran, and the operator can retry the uninstall. Record why the
		// delete moved on.
		failures := make([]string, 0, len(remaining))
		for _, name := range remaining {
			failures = append(failures, name+": uninstall did not finish")
		}
		st.Failures = failures
		st.Phase = fleetv1alpha1.AppsCleanupPhaseFailed
		st.Message = fmt.Sprintf(
			"%d application(s) still uninstalling after %s; left in Deleting",
			len(remaining), appsCleanupTimeout)
		return true, 0, save()
	}

	st.Message = fmt.Sprintf("uninstalling applications (%d/%d)", len(completed), len(st.Apps))
	return false, waitAfter, save()
}

// clusterAppNames lists the apps installed on a cluster.
func (r *ClusterJoinReconciler) clusterAppNames(ctx context.Context, clusterName string) ([]string, error) {
	var list fleetv1alpha1.FleetAppList
	if err := r.List(ctx, &list, client.InNamespace(r.Cfg.Namespace)); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list.Items))
	for i := range list.Items {
		if strings.TrimSpace(list.Items[i].Spec.ClusterName) != clusterName {
			continue
		}
		names = append(names, list.Items[i].Name)
	}
	return names, nil
}

// appsCleanupMessage renders the delete-stage status line while applications are
// being uninstalled.
func appsCleanupMessage(fcj *fleetv1alpha1.FleetClusterJoin) string {
	st := fcj.Status.AppsCleanup
	if st == nil {
		return "Uninstalling cluster applications"
	}
	msg := fmt.Sprintf("Uninstalling cluster applications (%d/%d)", len(st.Completed), len(st.Apps))
	if strings.TrimSpace(st.Message) != "" {
		msg += ": " + st.Message
	}
	return msg
}
