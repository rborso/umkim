package reconcile

import (
	"context"
	"fmt"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/teardown"
	"github.com/rborso/umkim/controllers/fleet-site/internal/wg"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	annotationWGPeersRemoved       = "fleet.umkim.com/wg-peers-removed"
	annotationCleanupKubernetes    = "fleet.umkim.com/cleanup-kubernetes"
)

func wgPeersRemoved(fcj *fleetv1alpha1.FleetClusterJoin) bool {
	return fcj.Annotations != nil && fcj.Annotations[annotationWGPeersRemoved] == "true"
}

func clusterCleanupKubernetes(fcj *fleetv1alpha1.FleetClusterJoin) bool {
	return fcj.Annotations != nil && fcj.Annotations[annotationCleanupKubernetes] == "true"
}

func (r *ClusterJoinReconciler) clusterNeedsDeleteJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) bool {
	if teardown.ManagedClusterExists(ctx, r.Client, clusterName) {
		return true
	}
	if fcj.Status.ProvisionJobName != "" {
		return true
	}
	if fcj.Status.DeleteJobName != "" {
		return true
	}
	// Detach enrolled nodes (bootstrap/WG records) even when MC already removed.
	if _, err := r.clusterNodeWG(ctx, fcj, clusterName); err == nil {
		return true
	}
	// Hub-only: provider deregistration + ManifestWork cleanup.
	return true
}

func (r *ClusterJoinReconciler) markWGPeersRemoved(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin) error {
	return r.patchClusterJoin(ctx, fcj, func(latest *fleetv1alpha1.FleetClusterJoin) {
		if latest.Annotations == nil {
			latest.Annotations = map[string]string{}
		}
		latest.Annotations[annotationWGPeersRemoved] = "true"
	})
}

func (r *ClusterJoinReconciler) patchClusterJoin(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, mutate func(*fleetv1alpha1.FleetClusterJoin)) error {
	key := client.ObjectKeyFromObject(fcj)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		orig := latest.DeepCopy()
		mutate(&latest)
		return r.Patch(ctx, &latest, client.MergeFrom(orig))
	})
}

func (r *ClusterJoinReconciler) patchClusterJoinStatus(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, mutate func(*fleetv1alpha1.FleetClusterJoin)) error {
	key := client.ObjectKeyFromObject(fcj)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		orig := latest.DeepCopy()
		mutate(&latest)
		return r.Status().Patch(ctx, &latest, client.MergeFrom(orig))
	})
}

func (r *ClusterJoinReconciler) reconcileClusterDelete(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseDeleting {
		if err := r.patchClusterJoinStatus(ctx, fcj, func(latest *fleetv1alpha1.FleetClusterJoin) {
			latest.Status.Phase = fleetv1alpha1.JoinPhaseDeleting
			if latest.Status.DeletionStartedAt == nil {
				now := metav1.Now()
				latest.Status.DeletionStartedAt = &now
			}
			latest.Status.Message = "Cluster deletion in progress"
		}); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Run hub/spoke delete while overlay WG is still up so klusterlet can finish ManifestWork cleanup.
	needsDeleteJob := r.clusterNeedsDeleteJob(ctx, fcj, clusterName)
	if needsDeleteJob && fcj.Status.DeleteJobName == "" {
		if err := r.startClusterDelete(ctx, fcj, clusterName); err != nil {
			return r.failClusterDelete(ctx, fcj, err)
		}
		return ctrl.Result{Requeue: true}, nil
	}
	if fcj.Status.DeleteJobName != "" {
		done, ok, msg := r.jobDoneAllowMissing(ctx, fcj.Namespace, fcj.Status.DeleteJobName, false)
		if !done {
			var job batchv1.Job
			if err := r.Get(ctx, types.NamespacedName{Namespace: fcj.Namespace, Name: fcj.Status.DeleteJobName}, &job); apierrors.IsNotFound(err) {
				fcj.Status.DeleteJobName = ""
				fcj.Status.Message = "Cluster delete job missing; retrying"
				if err := r.Status().Update(ctx, fcj); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		fcj.Status.DeleteJobName = ""
		if !ok {
			if teardown.ManagedClusterExists(ctx, r.Client, clusterName) {
				fcj.Status.Message = fmt.Sprintf("Cluster delete job failed (%s); retrying", msg)
				if err := r.Status().Update(ctx, fcj); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			fcj.Status.Message = "Cluster delete job failed after hub unjoin; continuing teardown"
		}
		if err := r.Status().Update(ctx, fcj); err != nil {
			return ctrl.Result{}, err
		}
	}

	peers, _ := r.clusterWGPeers(ctx, fcj, clusterName)
	cleanupKubernetes := clusterCleanupKubernetes(fcj)
	if len(peers) > 0 && !wgPeersRemoved(fcj) && fcj.Status.WgRemoveJobName == "" {
		if !cleanupKubernetes && teardown.ManagedClusterExists(ctx, r.Client, clusterName) {
			// Detach keeps ManagedCluster + preserved ManifestWorks; proceed with WG teardown.
		} else if teardown.ManagedClusterExists(ctx, r.Client, clusterName) {
			fcj.Status.Message = "Waiting for ManagedCluster removal before WireGuard peer teardown"
			if err := r.Status().Update(ctx, fcj); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		if err := r.startWgRemove(ctx, fcj, clusterName, peers); err != nil {
			return r.failClusterDelete(ctx, fcj, err)
		}
		return ctrl.Result{Requeue: true}, nil
	}
	if fcj.Status.WgRemoveJobName != "" {
		done, ok, msg := r.jobDoneAllowMissing(ctx, fcj.Namespace, fcj.Status.WgRemoveJobName, true)
		if !done {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		if !ok {
			return r.failClusterDelete(ctx, fcj, fmt.Errorf("wireguard peer removal failed: %s", msg))
		}
		fcj.Status.WgRemoveJobName = ""
		fcj.Status.Message = "WireGuard peers removed"
		if err := r.Status().Update(ctx, fcj); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.markWGPeersRemoved(ctx, fcj); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := teardown.RemoveEdgeClusterEntry(ctx, r.Client, r.Cfg, clusterName); err != nil {
		return r.failClusterDelete(ctx, fcj, err)
	}
	if err := teardown.ReconcileDomains(ctx, r.Client, r.Cfg); err != nil {
		return r.failClusterDelete(ctx, fcj, err)
	}

	if err := teardown.DeleteClusterJoinArtifacts(ctx, r.Client, r.Cfg, clusterName); err != nil {
		return r.failClusterDelete(ctx, fcj, err)
	}

	controllerutil.RemoveFinalizer(fcj, fleetv1alpha1.ClusterCleanupFinalizer)
	if err := r.Update(ctx, fcj); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Delete(ctx, fcj); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ClusterJoinReconciler) clusterWGPeers(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) ([]jobs.WGPeer, error) {
	nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
	if err != nil {
		return nil, err
	}
	peers := make([]jobs.WGPeer, 0, len(nodeWG))
	for _, n := range nodeWG {
		if n.PublicKey == "" {
			continue
		}
		peers = append(peers, jobs.WGPeer{PublicKey: n.PublicKey, AllowedIP: wg.HostIP(n.WGAddress)})
	}
	return peers, nil
}

func (r *ClusterJoinReconciler) startWgRemove(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string, peers []jobs.WGPeer) error {
	jobName := fmt.Sprintf("fleet-cluster-wg-rm-%s-%d", clusterName, time.Now().Unix())
	j := jobs.WgRemoveJobMulti(fcj.Namespace, jobName, peers)
	if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
		return err
	}
	_ = r.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: fcj.Namespace}},
		client.PropagationPolicy(metav1.DeletePropagationForeground))
	if err := r.Create(ctx, j); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	fcj.Status.WgRemoveJobName = jobName
	fcj.Status.Message = "Removing WireGuard peers on hub"
	return r.Status().Update(ctx, fcj)
}

func (r *ClusterJoinReconciler) startClusterDelete(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) error {
	nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
	if err != nil {
		if teardown.ManagedClusterExists(ctx, r.Client, clusterName) {
			return err
		}
		nodeWG = nil
	}
	if err := r.ensureClusterInventoryCM(ctx, fcj, clusterName, nodeWG); err != nil {
		return err
	}
	if _, err := r.ensureClusterSSHSecret(ctx, fcj, clusterName); err != nil {
		return err
	}
	jobName := "fleet-cluster-delete-" + clusterName
	var existing batchv1.Job
	if err := r.Get(ctx, types.NamespacedName{Namespace: fcj.Namespace, Name: jobName}, &existing); err == nil {
		if done, _ := jobIsFinished(&existing); !done {
			fcj.Status.DeleteJobName = jobName
			fcj.Status.Message = "Running cluster delete (Ansible)"
			return r.Status().Update(ctx, fcj)
		}
		_ = r.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: fcj.Namespace}},
			client.PropagationPolicy(metav1.DeletePropagationForeground))
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	hubOnly := len(nodeWG) == 0
	cleanupKubernetes := clusterCleanupKubernetes(fcj)
	j := jobs.DeleteJob(fcj.Namespace, clusterName, fcj.Spec.SiteId, r.Cfg.AnsibleRunnerImage,
		clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName), hubOnly, cleanupKubernetes)
	j.Name = jobName
	if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, j); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	fcj.Status.DeleteJobName = jobName
	fcj.Status.Message = "Running cluster delete (Ansible)"
	return r.Status().Update(ctx, fcj)
}

func (r *ClusterJoinReconciler) failClusterDelete(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, err error) (ctrl.Result, error) {
	fcj.Status.Phase = fleetv1alpha1.JoinPhaseFailed
	fcj.Status.Message = "Delete failed: " + err.Error()
	_ = r.Status().Update(ctx, fcj)
	return ctrl.Result{}, err
}
