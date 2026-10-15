package reconcile

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingressauth"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *IngressAuthReconciler) maybeMigrateLegacyIngressAuth(ctx context.Context, fia *fleetv1alpha1.FleetIngressAuth) (bool, error) {
	if fia.Spec.HubEnabled || fia.Status.ObservedGeneration > 0 {
		return false, nil
	}
	ok, err := ingressauth.LegacyStackDeployedSafe(ctx, r.Client)
	if err != nil || !ok {
		return false, err
	}
	fia.Spec.HubEnabled = true
	if fia.Annotations == nil {
		fia.Annotations = map[string]string{}
	}
	fia.Annotations[ingressauth.ReconcileAnnotation] = "legacy-ansible-migration"
	if err := r.Update(ctx, fia); err != nil {
		return false, err
	}
	return true, nil
}

type IngressAuthReconciler struct {
	*Reconciler
}

func (r *IngressAuthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	ns := r.Cfg.Namespace

	fia, err := ingressauth.EnsureCR(ctx, r.Client, ns, r.Cfg)
	if err != nil {
		return ctrl.Result{}, err
	}
	if migrated, err := r.maybeMigrateLegacyIngressAuth(ctx, fia); err != nil {
		return ctrl.Result{}, err
	} else if migrated {
		return ctrl.Result{Requeue: true}, nil
	}
	if synced, err := r.maybeSyncInventoryIngressAuth(ctx, fia); err != nil {
		return ctrl.Result{}, err
	} else if synced {
		return ctrl.Result{Requeue: true}, nil
	}
	if req.Name != "" && req.Name != fleetv1alpha1.FleetIngressAuthName {
		if err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: fleetv1alpha1.FleetIngressAuthName}, fia); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	desired, err := ingressauth.ComputeDesired(ctx, r.Client, r.Cfg, fia)
	if err != nil {
		return r.failIngressAuth(ctx, fia, err)
	}
	for name, cc := range desired.ClusterConfigs {
		if len(cc.OIDCClients) == 0 {
			continue
		}
		ids := make([]string, 0, len(cc.OIDCClients))
		for _, oc := range cc.OIDCClients {
			ids = append(ids, oc.ClientID)
		}
		if _, err := ingressauth.EnsureOIDCSecrets(ctx, r.Client, r.Cfg.Namespace, name, ids); err != nil {
			return r.failIngressAuth(ctx, fia, err)
		}
	}

	if err := ingressauth.ReconcileHubLLDAPWebUI(ctx, r.Client, r.Cfg, fia); err != nil {
		logger.Error(err, "reconcile hub LLDAP web UI")
	}

	if fia.Status.Phase == fleetv1alpha1.IngressAuthPhaseApplying && fia.Status.LastJobName != "" {
		return r.pollIngressAuthJob(ctx, fia, desired)
	}

	if !r.needsIngressAuthReconcile(fia, desired) {
		if synced, err := r.syncStuckFCJIngressAuth(ctx, fia, desired); err != nil {
			return ctrl.Result{}, err
		} else if synced {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
	}

	if !desired.GlobalEnabled && !fia.Spec.HubEnabled && len(desired.EnabledClusters) == 0 && len(desired.TeardownClusters) == 0 {
		if fia.Status.Phase != fleetv1alpha1.IngressAuthPhaseDisabled {
			fia.Status.Phase = fleetv1alpha1.IngressAuthPhaseDisabled
			fia.Status.Message = "Ingress auth disabled"
			fia.Status.Hub.Phase = fleetv1alpha1.IngressAuthClusterPhaseDisabled
			fia.Status.Clusters = nil
			fia.Status.ObservedGeneration = fia.Generation
			_ = r.Status().Update(ctx, fia)
		}
		if fia.Annotations != nil {
			delete(fia.Annotations, ingressauth.ReconcileAnnotation)
			_ = r.Update(ctx, fia)
		}
		return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
	}

	if err := ingressauth.EnsureSecret(ctx, r.Client, ns, fia.Spec.EffectiveSecretsRef()); err != nil {
		return r.failIngressAuth(ctx, fia, err)
	}
	if err := ingressauth.EnsureInventoryCM(ctx, r.Client, ns, desired, r.Cfg, fia); err != nil {
		return r.failIngressAuth(ctx, fia, err)
	}

	jobName := fmt.Sprintf("fleet-ingress-auth-%d", time.Now().Unix())
	job := jobs.IngressAuthReconcileJob(ns, jobName, r.Cfg.AnsibleRunnerImage, ingressauth.InventoryCMName, fia.Spec.EffectiveSecretsRef(), desired.EnabledClusters)
	if err := controllerutil.SetControllerReference(fia, job, r.Scheme); err != nil {
		return r.failIngressAuth(ctx, fia, err)
	}
	if err := r.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
		return r.failIngressAuth(ctx, fia, err)
	}

	fia.Status.Phase = fleetv1alpha1.IngressAuthPhaseApplying
	fia.Status.Message = "Applying ingress auth"
	fia.Status.LastJobName = jobName
	fia.Status.Hub.Phase = fleetv1alpha1.IngressAuthClusterPhaseApplying
	if desired.HubEnabled {
		fia.Status.Hub.AutheliaServiceURL = "http://authelia.fleet-ingress-auth.svc.cluster.local:9091"
	}
	fia.Status.Clusters = clusterStatusApplying(desired)
	if err := r.Status().Update(ctx, fia); err != nil {
		return ctrl.Result{}, err
	}
	if fia.Annotations != nil {
		delete(fia.Annotations, ingressauth.ReconcileAnnotation)
		if err := r.Update(ctx, fia); err != nil {
			return ctrl.Result{}, err
		}
	}
	if err := r.updateFCJIngressAuthPhases(ctx, desired, fleetv1alpha1.IngressAuthClusterPhaseApplying, jobName); err != nil {
		logger.Error(err, "update FCJ ingress auth status")
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *IngressAuthReconciler) needsIngressAuthReconcile(fia *fleetv1alpha1.FleetIngressAuth, desired ingressauth.DesiredState) bool {
	if fia.Annotations != nil {
		if _, ok := fia.Annotations[ingressauth.ReconcileAnnotation]; ok {
			return true
		}
	}
	if ingressauth.AnsibleSpecChanged(fia, desired) {
		return true
	}
	if desired.GlobalEnabled && fia.Status.Phase != fleetv1alpha1.IngressAuthPhaseReady && fia.Status.Phase != fleetv1alpha1.IngressAuthPhaseApplying {
		return true
	}
	if !desired.GlobalEnabled && fia.Status.Phase != fleetv1alpha1.IngressAuthPhaseDisabled && fia.Status.Phase != "" {
		return true
	}
	return false
}

func (r *IngressAuthReconciler) pollIngressAuthJob(ctx context.Context, fia *fleetv1alpha1.FleetIngressAuth, desired ingressauth.DesiredState) (ctrl.Result, error) {
	var job batchv1.Job
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: fia.Status.LastJobName}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			return r.completeIngressAuth(ctx, fia, desired, nil)
		}
		return ctrl.Result{}, err
	}
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == "True" {
			return r.completeIngressAuth(ctx, fia, desired, appliedClusters(&job))
		}
		if c.Type == batchv1.JobFailed && c.Status == "True" {
			return r.failIngressAuth(ctx, fia, fmt.Errorf("job %s failed", job.Name))
		}
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

// appliedClusters returns the enabled clusters a reconcile Job rendered, or nil
// when the Job predates the annotation. Nil means "unknown", and the caller falls
// back to the current desired state (the pre-annotation behaviour).
func appliedClusters(job *batchv1.Job) []string {
	raw := strings.TrimSpace(job.Annotations[jobs.AppliedClustersAnnotation])
	if raw == "" {
		return nil
	}
	out := []string{}
	for _, name := range strings.Split(raw, ",") {
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// enabledSubset returns desired restricted to the named clusters (sorted). It is
// used to act on exactly the clusters a Job rendered, leaving any cluster enabled
// while that Job ran untouched for the follow-up reconcile.
func enabledSubset(desired ingressauth.DesiredState, names []string) ingressauth.DesiredState {
	out := desired
	out.EnabledClusters = append([]string(nil), names...)
	sort.Strings(out.EnabledClusters)
	return out
}

// completeIngressAuth records a successful reconcile. `applied` is the set of
// clusters the finished Job actually rendered; only those are marked ready and
// only those count towards the stored spec hash. A cluster enabled while the Job
// was running therefore stays pending, the hash keeps differing from the current
// desired state, and the next reconcile deploys it. Marking the whole current
// desired state ready instead was the bug: the cluster read `Ready` with no spoke
// stack, and the stored hash matched, so it was never deployed.
func (r *IngressAuthReconciler) completeIngressAuth(ctx context.Context, fia *fleetv1alpha1.FleetIngressAuth, desired ingressauth.DesiredState, applied []string) (ctrl.Result, error) {
	rendered := desired
	if applied != nil {
		rendered = enabledSubset(desired, applied)
	}
	if desired.GlobalEnabled {
		fia.Status.Phase = fleetv1alpha1.IngressAuthPhaseReady
		fia.Status.Message = "Ingress auth ready"
	} else {
		fia.Status.Phase = fleetv1alpha1.IngressAuthPhaseDisabled
		fia.Status.Message = "Ingress auth disabled"
	}
	fia.Status.ObservedGeneration = fia.Generation
	if desired.HubEnabled {
		fia.Status.Hub.Phase = fleetv1alpha1.IngressAuthClusterPhaseReady
		fia.Status.Hub.AutheliaServiceURL = "http://authelia.fleet-ingress-auth.svc.cluster.local:9091"
	} else {
		fia.Status.Hub.Phase = fleetv1alpha1.IngressAuthClusterPhaseDisabled
		fia.Status.Hub.AutheliaServiceURL = ""
		fia.Status.Hub.Message = ""
	}
	fia.Status.Clusters = clusterStatuses(desired, rendered)
	if err := r.Status().Update(ctx, fia); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.patchAnsibleSpecHash(ctx, fia, rendered); err != nil {
		return ctrl.Result{}, err
	}
	phase := fleetv1alpha1.IngressAuthClusterPhaseDisabled
	if desired.GlobalEnabled {
		phase = fleetv1alpha1.IngressAuthClusterPhaseReady
	}
	if err := r.updateFCJIngressAuthPhases(ctx, rendered, phase, ""); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
}

func (r *IngressAuthReconciler) patchAnsibleSpecHash(ctx context.Context, fia *fleetv1alpha1.FleetIngressAuth, desired ingressauth.DesiredState) error {
	var current fleetv1alpha1.FleetIngressAuth
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: fleetv1alpha1.FleetIngressAuthName}, &current); err != nil {
		return err
	}
	orig := current.DeepCopy()
	if current.Annotations == nil {
		current.Annotations = map[string]string{}
	}
	current.Annotations[ingressauth.AnsibleSpecHashAnnotation] = ingressauth.AnsibleSpecHash(fia, desired)
	return r.Patch(ctx, &current, client.MergeFrom(orig))
}

func (r *IngressAuthReconciler) failIngressAuth(ctx context.Context, fia *fleetv1alpha1.FleetIngressAuth, err error) (ctrl.Result, error) {
	fia.Status.Phase = fleetv1alpha1.IngressAuthPhaseFailed
	fia.Status.Message = err.Error()
	_ = r.Status().Update(ctx, fia)
	return ctrl.Result{}, err
}

func clusterStatusApplying(desired ingressauth.DesiredState) []fleetv1alpha1.FleetIngressAuthClusterStatus {
	out := make([]fleetv1alpha1.FleetIngressAuthClusterStatus, 0, len(desired.EnabledClusters))
	for _, name := range desired.EnabledClusters {
		out = append(out, fleetv1alpha1.FleetIngressAuthClusterStatus{
			Name:  name,
			Phase: fleetv1alpha1.IngressAuthClusterPhaseApplying,
		})
	}
	return out
}

// clusterStatuses reports every currently desired cluster: the ones the finished
// Job rendered are ready, and any cluster enabled while it ran is still applying.
func clusterStatuses(desired, rendered ingressauth.DesiredState) []fleetv1alpha1.FleetIngressAuthClusterStatus {
	ready := map[string]struct{}{}
	for _, name := range rendered.EnabledClusters {
		ready[name] = struct{}{}
	}
	names := append([]string(nil), desired.EnabledClusters...)
	sort.Strings(names)
	out := make([]fleetv1alpha1.FleetIngressAuthClusterStatus, 0, len(names))
	for _, name := range names {
		phase := fleetv1alpha1.IngressAuthClusterPhaseApplying
		if _, ok := ready[name]; ok {
			phase = fleetv1alpha1.IngressAuthClusterPhaseReady
		}
		out = append(out, fleetv1alpha1.FleetIngressAuthClusterStatus{
			Name:  name,
			Phase: phase,
		})
	}
	return out
}

func (r *IngressAuthReconciler) syncStuckFCJIngressAuth(ctx context.Context, fia *fleetv1alpha1.FleetIngressAuth, desired ingressauth.DesiredState) (bool, error) {
	if fia.Status.Phase != fleetv1alpha1.IngressAuthPhaseReady && fia.Status.Phase != fleetv1alpha1.IngressAuthPhaseFailed {
		return false, nil
	}
	var joins fleetv1alpha1.FleetClusterJoinList
	if err := r.List(ctx, &joins, client.InNamespace(r.Cfg.Namespace)); err != nil {
		return false, err
	}
	enabled := map[string]struct{}{}
	for _, n := range desired.EnabledClusters {
		enabled[n] = struct{}{}
	}
	var readySet, failedSet []string
	for i := range joins.Items {
		fcj := &joins.Items[i]
		if !fcj.Spec.IngressAuth.Enabled {
			continue
		}
		name := fcj.Spec.EffectiveClusterName(fcj.Name)
		if _, ok := enabled[name]; !ok {
			continue
		}
		if fcj.Status.IngressAuth.Phase != fleetv1alpha1.IngressAuthClusterPhaseApplying {
			continue
		}
		jobName := fcj.Status.IngressAuth.JobName
		if jobName == "" {
			jobName = fia.Status.LastJobName
		}
		if jobName == "" {
			continue
		}
		var job batchv1.Job
		if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: jobName}, &job); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return false, err
		}
		done, failed := ingressAuthJobFinished(&job)
		if !done {
			continue
		}
		// Only a cluster the Job actually rendered can be settled from its result;
		// one enabled after the Job started waits for the follow-up reconcile.
		if applied := appliedClusters(&job); applied != nil && !containsString(applied, name) {
			continue
		}
		if failed {
			failedSet = append(failedSet, name)
		} else {
			readySet = append(readySet, name)
		}
	}
	if len(readySet) == 0 && len(failedSet) == 0 {
		return false, nil
	}
	if len(readySet) > 0 {
		if err := r.updateFCJIngressAuthPhases(ctx, enabledSubset(desired, readySet), fleetv1alpha1.IngressAuthClusterPhaseReady, ""); err != nil {
			return false, err
		}
	}
	if len(failedSet) > 0 {
		if err := r.updateFCJIngressAuthPhases(ctx, enabledSubset(desired, failedSet), fleetv1alpha1.IngressAuthClusterPhaseFailed, ""); err != nil {
			return false, err
		}
	}
	return true, nil
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func ingressAuthJobFinished(job *batchv1.Job) (done bool, failed bool) {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == "True" {
			return true, false
		}
		if c.Type == batchv1.JobFailed && c.Status == "True" {
			return true, true
		}
	}
	return false, false
}

func (r *IngressAuthReconciler) updateFCJIngressAuthPhases(ctx context.Context, desired ingressauth.DesiredState, phase, jobName string) error {
	var joins fleetv1alpha1.FleetClusterJoinList
	if err := r.List(ctx, &joins, client.InNamespace(r.Cfg.Namespace)); err != nil {
		return err
	}
	enabled := map[string]struct{}{}
	for _, n := range desired.EnabledClusters {
		enabled[n] = struct{}{}
	}
	for i := range joins.Items {
		fcj := &joins.Items[i]
		name := fcj.Spec.EffectiveClusterName(fcj.Name)
		if !fcj.Spec.IngressAuth.Enabled {
			if fcj.Status.IngressAuth.Phase != "" && fcj.Status.IngressAuth.Phase != fleetv1alpha1.IngressAuthClusterPhaseDisabled {
				fcj.Status.IngressAuth = fleetv1alpha1.FleetClusterJoinIngressAuthStatus{
					Phase: fleetv1alpha1.IngressAuthClusterPhaseDisabled,
				}
				_ = r.Status().Update(ctx, fcj)
			}
			continue
		}
		if _, ok := enabled[name]; !ok {
			continue
		}
		fcj.Status.IngressAuth.Phase = phase
		fcj.Status.IngressAuth.JobName = jobName
		if phase == fleetv1alpha1.IngressAuthClusterPhaseReady {
			fcj.Status.IngressAuth.Message = "Ingress forward-auth active"
		}
		if err := r.Status().Update(ctx, fcj); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) SetupIngressAuthWithManager(mgr ctrl.Manager) error {
	iar := &IngressAuthReconciler{Reconciler: r}
	return ctrl.NewControllerManagedBy(mgr).
		For(&fleetv1alpha1.FleetIngressAuth{}).
		Owns(&batchv1.Job{}).
		Watches(&fleetv1alpha1.FleetClusterJoin{}, handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
			return []reconcile.Request{{NamespacedName: types.NamespacedName{
				Name:      fleetv1alpha1.FleetIngressAuthName,
				Namespace: obj.GetNamespace(),
			}}}
		})).
		Complete(iar)
}
