package reconcile

import (
	"context"
	"fmt"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/acme"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type AcmeReconciler struct {
	*Reconciler
}

func (r *AcmeReconciler) maybeMigrateLegacyAcme(ctx context.Context, fa *fleetv1alpha1.FleetAcme) (bool, error) {
	if fa.Spec.HubEnabled || fa.Status.ObservedGeneration > 0 {
		return false, nil
	}
	ok, err := acme.LegacyHubACMEEnabled(ctx, r.Client)
	if err != nil || !ok {
		return false, err
	}
	fa.Spec.HubEnabled = true
	if fa.Annotations == nil {
		fa.Annotations = map[string]string{}
	}
	fa.Annotations[acme.ReconcileAnnotation] = "legacy-ansible-migration"
	if err := r.Update(ctx, fa); err != nil {
		return false, err
	}
	return true, nil
}

func (r *AcmeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	ns := r.Cfg.Namespace

	fa, err := acme.EnsureCR(ctx, r.Client, ns, r.Cfg)
	if err != nil {
		return ctrl.Result{}, err
	}
	if migrated, err := r.maybeMigrateLegacyAcme(ctx, fa); err != nil {
		return ctrl.Result{}, err
	} else if migrated {
		return ctrl.Result{Requeue: true}, nil
	}
	if synced, err := r.maybeSyncInventoryAcme(ctx, fa); err != nil {
		return ctrl.Result{}, err
	} else if synced {
		return ctrl.Result{Requeue: true}, nil
	}
	if req.Name != "" && req.Name != fleetv1alpha1.FleetAcmeName {
		if err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: fleetv1alpha1.FleetAcmeName}, fa); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	desired, err := acme.ComputeDesired(ctx, r.Client, r.Cfg, fa)
	if err != nil {
		return r.failAcme(ctx, fa, err)
	}

	if fa.Status.Phase == fleetv1alpha1.AcmePhaseApplying && fa.Status.LastJobName != "" {
		return r.pollAcmeJob(ctx, fa, desired)
	}

	if fa.Status.Phase == fleetv1alpha1.AcmePhaseReady &&
		fa.Annotations != nil && fa.Annotations[acme.ReconcileAnnotation] != "" &&
		!acme.AnsibleSpecChanged(fa, desired) {
		if err := r.patchAcmeMetadata(ctx, desired, true); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
	}

	if !r.needsAcmeReconcile(fa, desired) {
		if synced, err := r.syncStuckFCJAcme(ctx, fa, desired); err != nil {
			return ctrl.Result{}, err
		} else if synced {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
	}

	if !desired.GlobalEnabled {
		if fa.Status.Phase != fleetv1alpha1.AcmePhaseDisabled {
			fa.Status.Phase = fleetv1alpha1.AcmePhaseDisabled
			fa.Status.Message = "Let's Encrypt disabled"
			fa.Status.Hub.Phase = fleetv1alpha1.AcmeClusterPhaseDisabled
			fa.Status.Clusters = nil
			fa.Status.ObservedGeneration = fa.Generation
			_ = r.Status().Update(ctx, fa)
		}
		if fa.Annotations != nil {
			delete(fa.Annotations, acme.ReconcileAnnotation)
			_ = r.Update(ctx, fa)
		}
		return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
	}

	if err := acme.EnsureInventoryCM(ctx, r.Client, ns, desired, r.Cfg, fa); err != nil {
		return r.failAcme(ctx, fa, err)
	}

	if active, err := r.findActiveAcmeJob(ctx, ns); err != nil {
		return ctrl.Result{}, err
	} else if active != nil {
		fa.Status.Phase = fleetv1alpha1.AcmePhaseApplying
		fa.Status.Message = "Applying Let's Encrypt configuration"
		fa.Status.LastJobName = active.Name
		fa.Status.Hub.Phase = fleetv1alpha1.AcmeClusterPhaseApplying
		_ = r.Status().Update(ctx, fa)
		return r.pollAcmeJob(ctx, fa, desired)
	}

	jobName := fmt.Sprintf("fleet-acme-%d", time.Now().Unix())
	job := jobs.AcmeReconcileJob(ns, jobName, r.Cfg.AnsibleRunnerImage, acme.InventoryCMName)
	if err := r.Create(ctx, job); err != nil {
		return r.failAcme(ctx, fa, err)
	}

	fa.Status.Phase = fleetv1alpha1.AcmePhaseApplying
	fa.Status.Message = "Applying Let's Encrypt configuration"
	fa.Status.LastJobName = jobName
	fa.Status.Hub.Phase = fleetv1alpha1.AcmeClusterPhaseApplying
	fa.Status.Clusters = clusterAcmeStatusApplying(desired)
	fa.Status.ObservedGeneration = fa.Generation
	if err := r.Status().Update(ctx, fa); err != nil {
		logger.Error(err, "update acme status")
	}
	if err := r.updateFCJAcmePhases(ctx, desired, fleetv1alpha1.AcmeClusterPhaseApplying, jobName); err != nil {
		logger.Error(err, "update fcj acme phases")
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *AcmeReconciler) needsAcmeReconcile(fa *fleetv1alpha1.FleetAcme, desired acme.DesiredState) bool {
	if fa.Status.Phase == fleetv1alpha1.AcmePhaseApplying {
		return false
	}
	if fa.Annotations != nil && fa.Annotations[acme.ReconcileAnnotation] != "" {
		return true
	}
	if acme.AnsibleSpecChanged(fa, desired) {
		return true
	}
	if desired.GlobalEnabled && fa.Status.Phase != fleetv1alpha1.AcmePhaseReady && fa.Status.Phase != fleetv1alpha1.AcmePhaseApplying {
		return true
	}
	if !desired.GlobalEnabled && fa.Status.Phase != fleetv1alpha1.AcmePhaseDisabled && fa.Status.Phase != "" {
		return true
	}
	return false
}

func (r *AcmeReconciler) pollAcmeJob(ctx context.Context, fa *fleetv1alpha1.FleetAcme, desired acme.DesiredState) (ctrl.Result, error) {
	var job batchv1.Job
	if err := r.Get(ctx, types.NamespacedName{Namespace: fa.Namespace, Name: fa.Status.LastJobName}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			if active, aerr := r.findActiveAcmeJob(ctx, fa.Namespace); aerr != nil {
				return ctrl.Result{}, aerr
			} else if active != nil {
				fa.Status.LastJobName = active.Name
				_ = r.Status().Update(ctx, fa)
				return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
			}
			return r.completeAcme(ctx, fa, desired)
		}
		return ctrl.Result{}, err
	}
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == "True" {
			return r.completeAcme(ctx, fa, desired)
		}
		if c.Type == batchv1.JobFailed && c.Status == "True" {
			return r.failAcme(ctx, fa, fmt.Errorf("job %s failed", job.Name))
		}
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *AcmeReconciler) completeAcme(ctx context.Context, fa *fleetv1alpha1.FleetAcme, desired acme.DesiredState) (ctrl.Result, error) {
	if desired.GlobalEnabled {
		fa.Status.Phase = fleetv1alpha1.AcmePhaseReady
		fa.Status.Message = "Let's Encrypt active"
	} else {
		fa.Status.Phase = fleetv1alpha1.AcmePhaseDisabled
		fa.Status.Message = "Let's Encrypt disabled"
	}
	if desired.HubEnabled {
		fa.Status.Hub.Phase = fleetv1alpha1.AcmeClusterPhaseReady
		fa.Status.Hub.Message = "Hub gateway ACME active"
	} else {
		fa.Status.Hub.Phase = fleetv1alpha1.AcmeClusterPhaseDisabled
	}
	fa.Status.Clusters = clusterAcmeStatusReady(desired)
	fa.Status.ObservedGeneration = fa.Generation
	if err := r.Status().Update(ctx, fa); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.patchAcmeMetadata(ctx, desired, true); err != nil {
		return ctrl.Result{}, err
	}
	phase := fleetv1alpha1.AcmeClusterPhaseDisabled
	if desired.GlobalEnabled {
		phase = fleetv1alpha1.AcmeClusterPhaseReady
	}
	if err := r.updateFCJAcmePhases(ctx, desired, phase, ""); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
}

func (r *AcmeReconciler) failAcme(ctx context.Context, fa *fleetv1alpha1.FleetAcme, err error) (ctrl.Result, error) {
	fa.Status.Phase = fleetv1alpha1.AcmePhaseFailed
	fa.Status.Message = err.Error()
	fa.Status.Hub.Phase = fleetv1alpha1.AcmeClusterPhaseFailed
	fa.Status.Hub.Message = err.Error()
	_ = r.Status().Update(ctx, fa)
	desired, derr := acme.ComputeDesired(ctx, r.Client, r.Cfg, fa)
	if derr == nil {
		_ = r.patchAcmeMetadata(ctx, desired, true)
	}
	return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, err
}

func (r *AcmeReconciler) patchAcmeMetadata(ctx context.Context, desired acme.DesiredState, clearReconcile bool) error {
	var current fleetv1alpha1.FleetAcme
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: fleetv1alpha1.FleetAcmeName}, &current); err != nil {
		return err
	}
	orig := current.DeepCopy()
	if current.Annotations == nil {
		current.Annotations = map[string]string{}
	}
	current.Annotations[acme.AnsibleSpecHashAnnotation] = acme.AnsibleSpecHash(desired)
	if clearReconcile {
		delete(current.Annotations, acme.ReconcileAnnotation)
	}
	return r.Patch(ctx, &current, client.MergeFrom(orig))
}

func (r *AcmeReconciler) findActiveAcmeJob(ctx context.Context, ns string) (*batchv1.Job, error) {
	var list batchv1.JobList
	if err := r.List(ctx, &list, client.InNamespace(ns), client.MatchingLabels(map[string]string{"app": "fleet-acme-ansible"})); err != nil {
		return nil, err
	}
	for i := range list.Items {
		j := &list.Items[i]
		if j.Status.Succeeded == 0 && j.Status.Failed == 0 {
			return j, nil
		}
	}
	return nil, nil
}

func clusterAcmeStatusApplying(desired acme.DesiredState) []fleetv1alpha1.FleetAcmeClusterStatus {
	out := make([]fleetv1alpha1.FleetAcmeClusterStatus, 0, len(desired.EnabledClusters))
	for _, name := range desired.EnabledClusters {
		out = append(out, fleetv1alpha1.FleetAcmeClusterStatus{
			Name:  name,
			Phase: fleetv1alpha1.AcmeClusterPhaseApplying,
		})
	}
	return out
}

func clusterAcmeStatusReady(desired acme.DesiredState) []fleetv1alpha1.FleetAcmeClusterStatus {
	out := make([]fleetv1alpha1.FleetAcmeClusterStatus, 0, len(desired.EnabledClusters))
	for _, name := range desired.EnabledClusters {
		out = append(out, fleetv1alpha1.FleetAcmeClusterStatus{
			Name:  name,
			Phase: fleetv1alpha1.AcmeClusterPhaseReady,
		})
	}
	return out
}

func (r *AcmeReconciler) syncStuckFCJAcme(ctx context.Context, fa *fleetv1alpha1.FleetAcme, desired acme.DesiredState) (bool, error) {
	if fa.Status.Phase != fleetv1alpha1.AcmePhaseReady && fa.Status.Phase != fleetv1alpha1.AcmePhaseFailed {
		return false, nil
	}
	var joins fleetv1alpha1.FleetClusterJoinList
	if err := r.List(ctx, &joins, client.InNamespace(r.Cfg.Namespace)); err != nil {
		return false, err
	}
	targetPhase := fleetv1alpha1.AcmeClusterPhaseReady
	if fa.Status.Phase == fleetv1alpha1.AcmePhaseFailed {
		targetPhase = fleetv1alpha1.AcmeClusterPhaseFailed
	}
	changed := false
	for i := range joins.Items {
		fcj := &joins.Items[i]
		if !fcj.Spec.Acme.Enabled {
			continue
		}
		if fcj.Status.Acme.Phase == targetPhase {
			continue
		}
		fcj.Status.Acme.Phase = targetPhase
		if targetPhase == fleetv1alpha1.AcmeClusterPhaseReady {
			fcj.Status.Acme.Message = "Cluster Let's Encrypt active"
		}
		if err := r.Status().Update(ctx, fcj); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

func (r *AcmeReconciler) updateFCJAcmePhases(ctx context.Context, desired acme.DesiredState, phase, jobName string) error {
	var joins fleetv1alpha1.FleetClusterJoinList
	if err := r.List(ctx, &joins, client.InNamespace(r.Cfg.Namespace)); err != nil {
		return err
	}
	enabled := map[string]struct{}{}
	for _, name := range desired.EnabledClusters {
		enabled[name] = struct{}{}
	}
	for i := range joins.Items {
		fcj := &joins.Items[i]
		name := fcj.Spec.EffectiveClusterName(fcj.Name)
		if !fcj.Spec.Acme.Enabled {
			if fcj.Status.Acme.Phase != "" && fcj.Status.Acme.Phase != fleetv1alpha1.AcmeClusterPhaseDisabled {
				fcj.Status.Acme = fleetv1alpha1.FleetClusterJoinAcmeStatus{
					Phase:   fleetv1alpha1.AcmeClusterPhaseDisabled,
					Message: "Cluster ACME disabled",
				}
				if err := r.Status().Update(ctx, fcj); err != nil {
					return err
				}
			}
			continue
		}
		if _, ok := enabled[name]; !ok {
			continue
		}
		fcj.Status.Acme.Phase = phase
		fcj.Status.Acme.JobName = jobName
		if phase == fleetv1alpha1.AcmeClusterPhaseReady {
			fcj.Status.Acme.Message = "Cluster Let's Encrypt active"
		}
		if err := r.Status().Update(ctx, fcj); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) SetupAcmeWithManager(mgr ctrl.Manager) error {
	ar := &AcmeReconciler{Reconciler: r}
	return ctrl.NewControllerManagedBy(mgr).
		For(&fleetv1alpha1.FleetAcme{}).
		Watches(
			&fleetv1alpha1.FleetClusterJoin{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
				return []reconcile.Request{{NamespacedName: types.NamespacedName{
					Namespace: obj.GetNamespace(),
					Name:      fleetv1alpha1.FleetAcmeName,
				}}}
			}),
		).
		Complete(ar)
}
