package reconcile

import (
	"context"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	kubeconfigSyncAnnotation = "fleet.umkim.com/kubeconfig-sync"
	kubeconfigSyncJobAnn     = "fleet.umkim.com/kubeconfig-sync-job"
	spokeKubeconfigSecretNS  = "fleet-system"
)

func fleetKubeconfigSecretName(clusterName string) string {
	return "fleet-kubeconfig-" + clusterName
}

func (r *ClusterJoinReconciler) spokeKubeconfigSecretReady(ctx context.Context, clusterName string) (bool, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: spokeKubeconfigSecretNS,
		Name:      fleetKubeconfigSecretName(clusterName),
	}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return len(secret.Data["kubeconfig"]) > 0, nil
}

func (r *ClusterJoinReconciler) triggerKubeconfigSync(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin) error {
	ready, err := r.spokeKubeconfigSecretReady(ctx, fcj.Spec.EffectiveClusterName(fcj.Name))
	if err != nil || ready {
		return err
	}
	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	if _, ok := fcj.Annotations[kubeconfigSyncAnnotation]; ok {
		return nil
	}
	if _, ok := fcj.Annotations[kubeconfigSyncJobAnn]; ok {
		return nil
	}
	fcj.Annotations[kubeconfigSyncAnnotation] = time.Now().UTC().Format(time.RFC3339)
	return r.Update(ctx, fcj)
}

func (r *ClusterJoinReconciler) reconcileKubeconfigSecret(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	avail, err := r.managedClusterAvailable(ctx, clusterName)
	if err != nil || !avail {
		return ctrl.Result{}, nil
	}
	ready, err := r.spokeKubeconfigSecretReady(ctx, clusterName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if ready {
		if fcj.Annotations != nil && (fcj.Annotations[kubeconfigSyncAnnotation] != "" || fcj.Annotations[kubeconfigSyncJobAnn] != "") {
			delete(fcj.Annotations, kubeconfigSyncAnnotation)
			delete(fcj.Annotations, kubeconfigSyncJobAnn)
			if err := r.Update(ctx, fcj); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	if _, ok := fcj.Annotations[kubeconfigSyncAnnotation]; !ok {
		if _, ok := fcj.Annotations[kubeconfigSyncJobAnn]; !ok {
			fcj.Annotations[kubeconfigSyncAnnotation] = time.Now().UTC().Format(time.RFC3339)
			if err := r.Update(ctx, fcj); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{Requeue: true}, nil
	}

	jobName := fcj.Annotations[kubeconfigSyncJobAnn]
	if jobName == "" {
		nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := r.ensureClusterInventoryCM(ctx, fcj, clusterName, nodeWG); err != nil {
			return ctrl.Result{}, err
		}
		if _, err := r.ensureClusterSSHSecret(ctx, fcj, clusterName); err != nil {
			return ctrl.Result{}, err
		}
		jobName = "fleet-cluster-kubeconfig-sync-" + clusterName
		j := jobs.KubeconfigSyncJob(fcj.Namespace, jobName, clusterName, fcj.Spec.SiteId,
			r.Cfg.AnsibleRunnerImage, clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName))
		if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, j); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, err
		}
		fcj.Annotations[kubeconfigSyncJobAnn] = jobName
		if err := r.Update(ctx, fcj); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	done, ok, _ := r.jobDone(ctx, fcj.Namespace, jobName)
	if !done {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	delete(fcj.Annotations, kubeconfigSyncAnnotation)
	delete(fcj.Annotations, kubeconfigSyncJobAnn)
	if err := r.Update(ctx, fcj); err != nil {
		return ctrl.Result{}, err
	}
	if !ok {
		fcj.Annotations[kubeconfigSyncAnnotation] = time.Now().UTC().Format(time.RFC3339)
		if err := r.Update(ctx, fcj); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	secretReady, err := r.spokeKubeconfigSecretReady(ctx, clusterName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !secretReady {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}
