package reconcile

import (
	"context"
	"fmt"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	configureGpuHostAnnotation    = "fleet.umkim.com/configure-gpu-host"
	configureGpuHostJobAnnotation = "fleet.umkim.com/configure-gpu-host-job"
	configureGpuEnableAnnotation  = "fleet.umkim.com/configure-gpu-enable"
	configureGpuEnableJobAnnotation = "fleet.umkim.com/configure-gpu-enable-job"
)

func (r *ClusterJoinReconciler) gpuHostJobBusy(fcj *fleetv1alpha1.FleetClusterJoin) bool {
	return fcj.Status.Gpu.HostPhase == fleetv1alpha1.GpuHostPhaseApplying && fcj.Status.Gpu.HostJobName != ""
}

func (r *ClusterJoinReconciler) gpuRuntimeJobBusy(fcj *fleetv1alpha1.FleetClusterJoin) bool {
	return fcj.Status.Gpu.RuntimePhase == fleetv1alpha1.GpuRuntimePhaseApplying && fcj.Status.Gpu.RuntimeJobName != ""
}

func (r *ClusterJoinReconciler) reconcileClusterGpu(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	if res, err := r.reconcileGpuHost(ctx, fcj, clusterName); err != nil || res.Requeue || res.RequeueAfter > 0 {
		return res, err
	}
	return r.reconcileGpuEnable(ctx, fcj, clusterName)
}

func (r *ClusterJoinReconciler) reconcileGpuHost(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	if fcj.Annotations == nil {
		if r.gpuHostJobBusy(fcj) {
			return r.pollGpuHostJob(ctx, fcj, clusterName)
		}
		return ctrl.Result{}, nil
	}
	if _, ok := fcj.Annotations[configureGpuHostAnnotation]; !ok {
		if r.gpuHostJobBusy(fcj) {
			return r.pollGpuHostJob(ctx, fcj, clusterName)
		}
		return ctrl.Result{}, nil
	}

	jobAnn := fcj.Annotations[configureGpuHostJobAnnotation]
	if jobAnn == "" {
		if r.gpuRuntimeJobBusy(fcj) || r.storageJobBusy(fcj) || r.kubevirtJobBusy(fcj) {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		if fcj.Status.NodeOperation != nil &&
			(fcj.Status.NodeOperation.Phase == fleetv1alpha1.NodeOpPhasePending ||
				fcj.Status.NodeOperation.Phase == fleetv1alpha1.NodeOpPhaseRunning) {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
		if err != nil {
			return r.failGpuHost(ctx, fcj, err)
		}
		if err := r.ensureClusterInventoryCM(ctx, fcj, clusterName, nodeWG); err != nil {
			return r.failGpuHost(ctx, fcj, err)
		}
		if _, err := r.ensureClusterSSHSecret(ctx, fcj, clusterName); err != nil {
			return r.failGpuHost(ctx, fcj, err)
		}
		jobName := "fleet-cluster-gpu-host-" + clusterName
		var existing batchv1.Job
		if err := r.Get(ctx, client.ObjectKey{Namespace: fcj.Namespace, Name: jobName}, &existing); err == nil {
			if done, ok := jobIsFinished(&existing); done {
				if ok {
					return r.finishGpuHostJob(ctx, fcj, true, "")
				}
				_ = r.Delete(ctx, &existing, client.PropagationPolicy(metav1.DeletePropagationForeground))
			} else {
				return r.markGpuHostApplying(ctx, fcj, jobName)
			}
		} else if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		j := jobs.ConfigureGpuHostJob(fcj.Namespace, jobName, clusterName, fcj.Spec.SiteId,
			r.Cfg.AnsibleRunnerImage, clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName))
		if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, j); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return r.markGpuHostApplying(ctx, fcj, jobName)
			}
			return r.failGpuHost(ctx, fcj, err)
		}
		return r.markGpuHostApplying(ctx, fcj, jobName)
	}
	return r.pollGpuHostJob(ctx, fcj, clusterName)
}

func (r *ClusterJoinReconciler) reconcileGpuEnable(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		return ctrl.Result{}, nil
	}
	if fcj.Annotations == nil {
		if r.gpuRuntimeJobBusy(fcj) {
			return r.pollGpuEnableJob(ctx, fcj, clusterName)
		}
		return ctrl.Result{}, nil
	}
	if _, ok := fcj.Annotations[configureGpuEnableAnnotation]; !ok {
		if r.gpuRuntimeJobBusy(fcj) {
			return r.pollGpuEnableJob(ctx, fcj, clusterName)
		}
		return ctrl.Result{}, nil
	}

	jobAnn := fcj.Annotations[configureGpuEnableJobAnnotation]
	if jobAnn == "" {
		if r.gpuHostJobBusy(fcj) {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
		if err != nil {
			return r.failGpuEnable(ctx, fcj, err)
		}
		if err := r.ensureClusterInventoryCM(ctx, fcj, clusterName, nodeWG); err != nil {
			return r.failGpuEnable(ctx, fcj, err)
		}
		if _, err := r.ensureClusterSSHSecret(ctx, fcj, clusterName); err != nil {
			return r.failGpuEnable(ctx, fcj, err)
		}
		jobName := "fleet-cluster-gpu-enable-" + clusterName
		var existing batchv1.Job
		if err := r.Get(ctx, client.ObjectKey{Namespace: fcj.Namespace, Name: jobName}, &existing); err == nil {
			if done, ok := jobIsFinished(&existing); done {
				if ok {
					return r.finishGpuEnableJob(ctx, fcj, true, "")
				}
				_ = r.Delete(ctx, &existing, client.PropagationPolicy(metav1.DeletePropagationForeground))
			} else {
				return r.markGpuEnableApplying(ctx, fcj, jobName)
			}
		} else if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		j := jobs.ConfigureGpuEnableJob(fcj.Namespace, jobName, clusterName, fcj.Spec.SiteId,
			r.Cfg.AnsibleRunnerImage, clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName))
		if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, j); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return r.markGpuEnableApplying(ctx, fcj, jobName)
			}
			return r.failGpuEnable(ctx, fcj, err)
		}
		return r.markGpuEnableApplying(ctx, fcj, jobName)
	}
	return r.pollGpuEnableJob(ctx, fcj, clusterName)
}

func (r *ClusterJoinReconciler) markGpuHostApplying(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, jobName string) (ctrl.Result, error) {
	key := client.ObjectKeyFromObject(fcj)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		if latest.Annotations == nil {
			latest.Annotations = map[string]string{}
		}
		latest.Annotations[configureGpuHostJobAnnotation] = jobName
		if err := r.Update(ctx, &latest); err != nil {
			return err
		}
		latest.Status.Gpu.HostPhase = fleetv1alpha1.GpuHostPhaseApplying
		latest.Status.Gpu.HostJobName = jobName
		latest.Status.Gpu.HostMessage = "Installing NVIDIA host driver"
		latest.Status.Message = latest.Status.Gpu.HostMessage
		return r.Status().Update(ctx, &latest)
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *ClusterJoinReconciler) markGpuEnableApplying(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, jobName string) (ctrl.Result, error) {
	key := client.ObjectKeyFromObject(fcj)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		if latest.Annotations == nil {
			latest.Annotations = map[string]string{}
		}
		latest.Annotations[configureGpuEnableJobAnnotation] = jobName
		if err := r.Update(ctx, &latest); err != nil {
			return err
		}
		latest.Status.Gpu.RuntimePhase = fleetv1alpha1.GpuRuntimePhaseApplying
		latest.Status.Gpu.RuntimeJobName = jobName
		latest.Status.Gpu.RuntimeMessage = "Enabling NVIDIA container runtime"
		latest.Status.Message = latest.Status.Gpu.RuntimeMessage
		return r.Status().Update(ctx, &latest)
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *ClusterJoinReconciler) pollGpuHostJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	jobName := fcj.Status.Gpu.HostJobName
	if jobName == "" && fcj.Annotations != nil {
		jobName = fcj.Annotations[configureGpuHostJobAnnotation]
	}
	if jobName == "" {
		return ctrl.Result{}, nil
	}
	done, ok, msg := r.jobDone(ctx, fcj.Namespace, jobName)
	if !done {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if !ok {
		return r.finishGpuHostJob(ctx, fcj, false, "GPU host prep failed: "+msg)
	}
	return r.finishGpuHostJob(ctx, fcj, true, "")
}

func (r *ClusterJoinReconciler) pollGpuEnableJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	jobName := fcj.Status.Gpu.RuntimeJobName
	if jobName == "" && fcj.Annotations != nil {
		jobName = fcj.Annotations[configureGpuEnableJobAnnotation]
	}
	if jobName == "" {
		return ctrl.Result{}, nil
	}
	done, ok, msg := r.jobDone(ctx, fcj.Namespace, jobName)
	if !done {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if !ok {
		return r.finishGpuEnableJob(ctx, fcj, false, "GPU runtime enable failed: "+msg)
	}
	return r.finishGpuEnableJob(ctx, fcj, true, "")
}

func (r *ClusterJoinReconciler) finishGpuHostJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, ok bool, message string) (ctrl.Result, error) {
	key := client.ObjectKeyFromObject(fcj)
	var latest fleetv1alpha1.FleetClusterJoin
	if err := r.Get(ctx, key, &latest); err != nil {
		return ctrl.Result{}, err
	}
	jobName := latest.Status.Gpu.HostJobName
	if jobName == "" && latest.Annotations != nil {
		jobName = latest.Annotations[configureGpuHostJobAnnotation]
	}
	logTail := r.captureNodeOpLogTail(ctx, latest.Namespace, jobName)
	finishedAt := metav1.Now()
	opPhase := fleetv1alpha1.NodeOpPhaseFailed
	if ok {
		opPhase = fleetv1alpha1.NodeOpPhaseSucceeded
	}
	if message == "" && ok {
		message = "NVIDIA host driver installed"
	}
	latest.Status.Gpu.HostJobName = ""
	if ok {
		latest.Status.Gpu.HostPhase = fleetv1alpha1.GpuHostPhaseReady
		latest.Status.Gpu.HostMessage = message
	} else {
		latest.Status.Gpu.HostPhase = fleetv1alpha1.GpuHostPhaseFailed
		latest.Status.Gpu.HostMessage = message
	}
	latest.Status.Message = latest.Status.Gpu.HostMessage
	latest.Status.NodeOperationHistory = prependNodeOpHistory(latest.Status.NodeOperationHistory, fleetv1alpha1.ClusterNodeOperationRecord{
		ID:         fmt.Sprintf("gpu-host-%d", finishedAt.UnixNano()),
		Type:       fleetv1alpha1.NodeOpTypeConfigureGpuHost,
		Phase:      opPhase,
		JobName:    jobName,
		Message:    message,
		FinishedAt: &finishedAt,
		LogTail:    logTail,
	})
	if err := r.Status().Update(ctx, &latest); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, key, &latest); err != nil {
		return ctrl.Result{}, err
	}
	if latest.Annotations == nil {
		latest.Annotations = map[string]string{}
	}
	delete(latest.Annotations, configureGpuHostAnnotation)
	delete(latest.Annotations, configureGpuHostJobAnnotation)
	if err := r.Update(ctx, &latest); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ClusterJoinReconciler) finishGpuEnableJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, ok bool, message string) (ctrl.Result, error) {
	key := client.ObjectKeyFromObject(fcj)
	var latest fleetv1alpha1.FleetClusterJoin
	if err := r.Get(ctx, key, &latest); err != nil {
		return ctrl.Result{}, err
	}
	jobName := latest.Status.Gpu.RuntimeJobName
	if jobName == "" && latest.Annotations != nil {
		jobName = latest.Annotations[configureGpuEnableJobAnnotation]
	}
	logTail := r.captureNodeOpLogTail(ctx, latest.Namespace, jobName)
	finishedAt := metav1.Now()
	opPhase := fleetv1alpha1.NodeOpPhaseFailed
	if ok {
		opPhase = fleetv1alpha1.NodeOpPhaseSucceeded
	}
	if message == "" && ok {
		message = "NVIDIA GPU runtime enabled"
	}
	latest.Status.Gpu.RuntimeJobName = ""
	if ok {
		latest.Status.Gpu.RuntimePhase = fleetv1alpha1.GpuRuntimePhaseReady
		latest.Status.Gpu.RuntimeMessage = message
	} else {
		latest.Status.Gpu.RuntimePhase = fleetv1alpha1.GpuRuntimePhaseFailed
		latest.Status.Gpu.RuntimeMessage = message
	}
	latest.Status.Message = latest.Status.Gpu.RuntimeMessage
	latest.Status.NodeOperationHistory = prependNodeOpHistory(latest.Status.NodeOperationHistory, fleetv1alpha1.ClusterNodeOperationRecord{
		ID:         fmt.Sprintf("gpu-enable-%d", finishedAt.UnixNano()),
		Type:       fleetv1alpha1.NodeOpTypeConfigureGpuEnable,
		Phase:      opPhase,
		JobName:    jobName,
		Message:    message,
		FinishedAt: &finishedAt,
		LogTail:    logTail,
	})
	if err := r.Status().Update(ctx, &latest); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, key, &latest); err != nil {
		return ctrl.Result{}, err
	}
	if latest.Annotations == nil {
		latest.Annotations = map[string]string{}
	}
	delete(latest.Annotations, configureGpuEnableAnnotation)
	delete(latest.Annotations, configureGpuEnableJobAnnotation)
	if ok {
		latest.Annotations[discoverAnnotation] = time.Now().UTC().Format(time.RFC3339)
	}
	if err := r.Update(ctx, &latest); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ClusterJoinReconciler) failGpuHost(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, err error) (ctrl.Result, error) {
	res, finishErr := r.finishGpuHostJob(ctx, fcj, false, fmt.Sprintf("GPU host prep failed: %v", err))
	if finishErr != nil {
		return res, finishErr
	}
	return res, err
}

func (r *ClusterJoinReconciler) failGpuEnable(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, err error) (ctrl.Result, error) {
	res, finishErr := r.finishGpuEnableJob(ctx, fcj, false, fmt.Sprintf("GPU runtime enable failed: %v", err))
	if finishErr != nil {
		return res, finishErr
	}
	return res, err
}
