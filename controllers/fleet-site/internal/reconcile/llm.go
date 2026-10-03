package reconcile

import (
	"context"
	"fmt"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/fleetllm"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/llm"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type LLMReconciler struct {
	*Reconciler
}

func (r *LLMReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	ns := r.Cfg.Namespace

	fl, err := fleetllm.EnsureCR(ctx, r.Client, ns, r.Cfg)
	if err != nil {
		return ctrl.Result{}, err
	}
	if synced, err := r.maybeSyncInventoryLLM(ctx, fl); err != nil {
		return ctrl.Result{}, err
	} else if synced {
		return ctrl.Result{Requeue: true}, nil
	}
	if req.Name != "" && req.Name != fleetv1alpha1.FleetLLMName {
		if err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: fleetv1alpha1.FleetLLMName}, fl); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	desired := fleetllm.ComputeDesired(ctx, r.Cfg, fl)

	if !fleetllm.BackendsManual(fl) {
		backends, err := llm.DiscoverBackendsFromFleetApps(ctx, r.Client, r.Cfg.EdgeClustersCMNamespace, r.Cfg.EdgeClustersCMName)
		if err == nil && !backendsEqual(fl.Spec.Backends, backends) {
			fl.Spec.Backends = backends
			if fl.Annotations == nil {
				fl.Annotations = map[string]string{}
			}
			fl.Annotations[fleetllm.BackendsManualAnnotation] = "false"
			fl.Annotations[fleetllm.ReconcileAnnotation] = time.Now().UTC().Format(time.RFC3339)
			if err := r.Update(ctx, fl); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
	}

	if fl.Status.Phase == fleetv1alpha1.LLMPhaseApplying && fl.Status.LastJobName != "" {
		return r.pollLLMJob(ctx, fl, desired)
	}

	if !r.needsLLMReconcile(fl, desired) {
		return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
	}

	if !desired.GlobalEnabled {
		if fl.Status.Phase != fleetv1alpha1.LLMPhaseDisabled {
			fl.Status.Phase = fleetv1alpha1.LLMPhaseDisabled
			fl.Status.Message = "LLM platform disabled"
			fl.Status.Hub.Phase = fleetv1alpha1.LLMPhaseDisabled
			fl.Status.Hub.Endpoint = ""
			fl.Status.ObservedGeneration = fl.Generation
			_ = r.Status().Update(ctx, fl)
		}
		if fl.Annotations != nil {
			delete(fl.Annotations, fleetllm.ReconcileAnnotation)
			_ = r.Update(ctx, fl)
		}
		return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
	}

	if err := fleetllm.EnsureInventoryCM(ctx, r.Client, ns, desired, r.Cfg, fl); err != nil {
		return r.failLLM(ctx, fl, err)
	}

	jobName := fmt.Sprintf("fleet-llm-%d", time.Now().Unix())
	job := jobs.LLMReconcileJob(ns, jobName, r.Cfg.AnsibleRunnerImage, fleetllm.InventoryCMName)
	if err := controllerutil.SetControllerReference(fl, job, r.Scheme); err != nil {
		return r.failLLM(ctx, fl, err)
	}
	if err := r.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
		return r.failLLM(ctx, fl, err)
	}

	fl.Status.Phase = fleetv1alpha1.LLMPhaseApplying
	fl.Status.Message = "Applying LLM platform"
	fl.Status.LastJobName = jobName
	fl.Status.Hub.Phase = fleetv1alpha1.LLMPhaseApplying
	if err := r.Status().Update(ctx, fl); err != nil {
		return ctrl.Result{}, err
	}
	if fl.Annotations != nil {
		delete(fl.Annotations, fleetllm.ReconcileAnnotation)
		if err := r.Update(ctx, fl); err != nil {
			return ctrl.Result{}, err
		}
	}
	logger.Info("started LLM reconcile job", "job", jobName)
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *LLMReconciler) needsLLMReconcile(fl *fleetv1alpha1.FleetLLM, desired fleetllm.DesiredState) bool {
	if fl.Annotations != nil {
		if _, ok := fl.Annotations[fleetllm.ReconcileAnnotation]; ok {
			return true
		}
	}
	if fl.Generation != fl.Status.ObservedGeneration {
		return true
	}
	if desired.GlobalEnabled && fl.Status.Phase != fleetv1alpha1.LLMPhaseReady && fl.Status.Phase != fleetv1alpha1.LLMPhaseApplying {
		return true
	}
	if !desired.GlobalEnabled && fl.Status.Phase != fleetv1alpha1.LLMPhaseDisabled {
		return true
	}
	if desired.GlobalEnabled && fl.Status.Phase == fleetv1alpha1.LLMPhaseReady {
		if fleetllm.BackendsHash(desired.Backends) != fleetllm.AppliedBackendsHash(fl) {
			return true
		}
	}
	return false
}

func (r *LLMReconciler) pollLLMJob(ctx context.Context, fl *fleetv1alpha1.FleetLLM, desired fleetllm.DesiredState) (ctrl.Result, error) {
	done, ok, msg := r.jobPollState(ctx, fl.Namespace, fl.Status.LastJobName, false)
	if !done {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	if ok {
		return r.completeLLM(ctx, fl, desired)
	}
	if r.llmHubOperational(ctx) {
		logger := log.FromContext(ctx)
		logger.Info("LLM reconcile job missing or failed but hub LiteLLM is operational", "job", fl.Status.LastJobName, "msg", msg)
		return r.completeLLM(ctx, fl, desired)
	}
	return r.failLLM(ctx, fl, fmt.Errorf("llm job failed: %s", msg))
}

func (r *LLMReconciler) llmHubOperational(ctx context.Context) bool {
	var dep appsv1.Deployment
	if err := r.Get(ctx, types.NamespacedName{Namespace: "fleet-llm", Name: "litellm"}, &dep); err != nil {
		return false
	}
	return dep.Status.ReadyReplicas >= 1
}

func (r *LLMReconciler) completeLLM(ctx context.Context, fl *fleetv1alpha1.FleetLLM, desired fleetllm.DesiredState) (ctrl.Result, error) {
	fl.Status.Phase = fleetv1alpha1.LLMPhaseReady
	fl.Status.Message = "LLM platform ready"
	fl.Status.Hub.Phase = fleetv1alpha1.LLMPhaseReady
	fl.Status.Hub.Message = ""
	fl.Status.Hub.Endpoint = fleetllm.HubEndpoint(desired.PublicHost)
	fl.Status.LastJobName = ""
	fl.Status.ObservedGeneration = fl.Generation
	if err := r.Status().Update(ctx, fl); err != nil {
		return ctrl.Result{}, err
	}
	if err := fleetllm.SetAppliedBackendsHash(ctx, r.Client, fl, desired.Backends); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
}

func (r *LLMReconciler) failLLM(ctx context.Context, fl *fleetv1alpha1.FleetLLM, err error) (ctrl.Result, error) {
	fl.Status.Phase = fleetv1alpha1.LLMPhaseFailed
	fl.Status.Message = err.Error()
	fl.Status.Hub.Phase = fleetv1alpha1.LLMPhaseFailed
	fl.Status.Hub.Message = err.Error()
	fl.Status.LastJobName = ""
	_ = r.Status().Update(ctx, fl)
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *LLMReconciler) maybeSyncInventoryLLM(ctx context.Context, fl *fleetv1alpha1.FleetLLM) (bool, error) {
	if !r.Cfg.FleetLLMEnabled {
		return false, nil
	}
	if fl.Spec.HubEnabled {
		return false, nil
	}
	fl.Spec.HubEnabled = true
	if fl.Annotations == nil {
		fl.Annotations = map[string]string{}
	}
	fl.Annotations[fleetllm.ReconcileAnnotation] = "inventory-sync"
	if err := r.Update(ctx, fl); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Reconciler) SetupLLMWithManager(mgr ctrl.Manager) error {
	lr := &LLMReconciler{Reconciler: r}
	return ctrl.NewControllerManagedBy(mgr).
		For(&fleetv1alpha1.FleetLLM{}).
		Owns(&batchv1.Job{}).
		Complete(lr)
}

func backendsEqual(a, b []fleetv1alpha1.FleetLLMBackend) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Model != b[i].Model || a[i].APIBase != b[i].APIBase {
			return false
		}
	}
	return true
}
