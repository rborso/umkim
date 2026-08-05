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
	configureStorageAnnotation    = "fleet.umkim.com/configure-storage"
	configureStorageJobAnnotation = "fleet.umkim.com/configure-storage-job"
)

func (r *ClusterJoinReconciler) storageJobBusy(fcj *fleetv1alpha1.FleetClusterJoin) bool {
	return fcj.Status.Storage.Phase == fleetv1alpha1.StoragePhaseApplying && fcj.Status.Storage.JobName != ""
}

func (r *ClusterJoinReconciler) reconcileClusterStorage(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		return ctrl.Result{}, nil
	}
	if fcj.Annotations == nil {
		if r.storageJobBusy(fcj) {
			return r.pollStorageJob(ctx, fcj, clusterName)
		}
		return ctrl.Result{}, nil
	}
	if _, ok := fcj.Annotations[configureStorageAnnotation]; !ok {
		if r.storageJobBusy(fcj) {
			return r.pollStorageJob(ctx, fcj, clusterName)
		}
		return ctrl.Result{}, nil
	}

	jobAnn := fcj.Annotations[configureStorageJobAnnotation]
	if jobAnn == "" {
		nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
		if err != nil {
			return r.failClusterStorage(ctx, fcj, err)
		}
		if err := r.ensureClusterInventoryCM(ctx, fcj, clusterName, nodeWG); err != nil {
			return r.failClusterStorage(ctx, fcj, err)
		}
		if _, err := r.ensureClusterSSHSecret(ctx, fcj, clusterName); err != nil {
			return r.failClusterStorage(ctx, fcj, err)
		}
		jobName := "fleet-cluster-storage-" + clusterName
		var existing batchv1.Job
		if err := r.Get(ctx, client.ObjectKey{Namespace: fcj.Namespace, Name: jobName}, &existing); err == nil {
			if done, ok := jobIsFinished(&existing); done {
				if ok {
					return r.finishStorageJob(ctx, fcj, true, "")
				}
				_ = r.Delete(ctx, &existing, client.PropagationPolicy(metav1.DeletePropagationForeground))
			} else {
				return r.markStorageJobApplying(ctx, fcj, jobName)
			}
		} else if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		j := jobs.ConfigureStorageJob(fcj.Namespace, jobName, clusterName, fcj.Spec.SiteId,
			r.Cfg.AnsibleRunnerImage, clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName))
		if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, j); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return r.markStorageJobApplying(ctx, fcj, jobName)
			}
			return r.failClusterStorage(ctx, fcj, err)
		}
		return r.markStorageJobApplying(ctx, fcj, jobName)
	}

	return r.pollStorageJob(ctx, fcj, clusterName)
}

func (r *ClusterJoinReconciler) markStorageJobApplying(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, jobName string) (ctrl.Result, error) {
	key := client.ObjectKeyFromObject(fcj)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		if latest.Annotations == nil {
			latest.Annotations = map[string]string{}
		}
		latest.Annotations[configureStorageJobAnnotation] = jobName
		if err := r.Update(ctx, &latest); err != nil {
			return err
		}
		latest.Status.Storage.Phase = fleetv1alpha1.StoragePhaseApplying
		latest.Status.Storage.JobName = jobName
		latest.Status.Storage.Message = "Configuring NFS storage"
		latest.Status.Message = latest.Status.Storage.Message
		return r.Status().Update(ctx, &latest)
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *ClusterJoinReconciler) pollStorageJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	jobName := fcj.Status.Storage.JobName
	if jobName == "" && fcj.Annotations != nil {
		jobName = fcj.Annotations[configureStorageJobAnnotation]
	}
	if jobName == "" {
		return ctrl.Result{}, nil
	}
	var job batchv1.Job
	if err := r.Get(ctx, client.ObjectKey{Namespace: fcj.Namespace, Name: jobName}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			// A concurrent reconcile may still be creating the job.
			if fcj.Annotations != nil && fcj.Annotations[configureStorageAnnotation] != "" {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			return r.finishStorageJob(ctx, fcj, false, "Storage configure job missing; retry from Console")
		}
		return ctrl.Result{}, err
	}
	done, ok, msg := r.jobDone(ctx, fcj.Namespace, jobName)
	if !done {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if !ok {
		return r.finishStorageJob(ctx, fcj, false, "Storage configure failed: "+msg)
	}
	return r.finishStorageJob(ctx, fcj, true, "")
}

func (r *ClusterJoinReconciler) finishStorageJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, ok bool, message string) (ctrl.Result, error) {
	key := client.ObjectKeyFromObject(fcj)
	var latest fleetv1alpha1.FleetClusterJoin
	if err := r.Get(ctx, key, &latest); err != nil {
		return ctrl.Result{}, err
	}

	latest.Status.Storage.JobName = ""
	if ok {
		latest.Status.Storage.Phase = fleetv1alpha1.StoragePhaseApplied
		latest.Status.Storage.Message = "NFS storage configured"
		latest.Status.Message = latest.Status.Storage.Message
	} else {
		latest.Status.Storage.Phase = fleetv1alpha1.StoragePhaseFailed
		latest.Status.Storage.Message = message
		latest.Status.Message = message
	}
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
	delete(latest.Annotations, configureStorageAnnotation)
	delete(latest.Annotations, configureStorageJobAnnotation)
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

func (r *ClusterJoinReconciler) failClusterStorage(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, err error) (ctrl.Result, error) {
	res, finishErr := r.finishStorageJob(ctx, fcj, false, fmt.Sprintf("Storage configure failed: %v", err))
	if finishErr != nil {
		return res, finishErr
	}
	return res, err
}
