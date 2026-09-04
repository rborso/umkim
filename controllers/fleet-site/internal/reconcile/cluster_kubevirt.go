package reconcile

import (
	"context"
	"fmt"
	"strings"
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
	configureKubevirtAnnotation    = "fleet.umkim.com/configure-kubevirt"
	configureKubevirtJobAnnotation = "fleet.umkim.com/configure-kubevirt-job"
)

func (r *ClusterJoinReconciler) kubevirtJobBusy(fcj *fleetv1alpha1.FleetClusterJoin) bool {
	st := fcj.Status.Providers[fleetv1alpha1.ProviderKindKubevirt]
	return st.Phase == fleetv1alpha1.ProviderPhaseInstalling && st.JobName != ""
}

func (r *ClusterJoinReconciler) reconcileClusterKubevirt(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		return ctrl.Result{}, nil
	}
	ensureProviderStatusDefaults(fcj, clusterName)

	// Auto-request when create intent set and not yet installed.
	if fcj.Spec.WantsProviderInstall(fleetv1alpha1.ProviderKindKubevirt) {
		st := fcj.Status.Providers[fleetv1alpha1.ProviderKindKubevirt]
		phase := providerStatusPhase(st)
		if phase == fleetv1alpha1.ProviderPhaseNotInstalled {
			if fcj.Annotations == nil {
				fcj.Annotations = map[string]string{}
			}
			if _, ok := fcj.Annotations[configureKubevirtAnnotation]; !ok {
				key := client.ObjectKeyFromObject(fcj)
				err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
					var latest fleetv1alpha1.FleetClusterJoin
					if err := r.Get(ctx, key, &latest); err != nil {
						return err
					}
					if latest.Annotations == nil {
						latest.Annotations = map[string]string{}
					}
					if _, ok := latest.Annotations[configureKubevirtAnnotation]; ok {
						return nil
					}
					latest.Annotations[configureKubevirtAnnotation] = time.Now().UTC().Format(time.RFC3339)
					if err := r.Update(ctx, &latest); err != nil {
						return err
					}
					ensureProviderStatusDefaults(&latest, clusterName)
					st := latest.Status.Providers[fleetv1alpha1.ProviderKindKubevirt]
					st.Phase = fleetv1alpha1.ProviderPhaseInstalling
					st.Message = "KubeVirt platform install requested"
					st.ProviderName = providerNameForKind(&latest, clusterName, fleetv1alpha1.ProviderKindKubevirt)
					latest.Status.Providers[fleetv1alpha1.ProviderKindKubevirt] = st
					latest.Status.Message = st.Message
					return r.Status().Update(ctx, &latest)
				})
				if err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{Requeue: true}, nil
			}
		}
	}

	if fcj.Annotations == nil {
		if r.kubevirtJobBusy(fcj) {
			return r.pollKubevirtJob(ctx, fcj, clusterName)
		}
		return ctrl.Result{}, nil
	}
	if _, ok := fcj.Annotations[configureKubevirtAnnotation]; !ok {
		if r.kubevirtJobBusy(fcj) {
			return r.pollKubevirtJob(ctx, fcj, clusterName)
		}
		return ctrl.Result{}, nil
	}

	jobAnn := fcj.Annotations[configureKubevirtJobAnnotation]
	if jobAnn == "" {
		if r.storageJobBusy(fcj) {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		if fcj.Status.NodeOperation != nil &&
			(fcj.Status.NodeOperation.Phase == fleetv1alpha1.NodeOpPhasePending ||
				fcj.Status.NodeOperation.Phase == fleetv1alpha1.NodeOpPhaseRunning) {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
		if err != nil {
			return r.failClusterKubevirt(ctx, fcj, err)
		}
		if err := r.ensureClusterInventoryCM(ctx, fcj, clusterName, nodeWG); err != nil {
			return r.failClusterKubevirt(ctx, fcj, err)
		}
		if _, err := r.ensureClusterSSHSecret(ctx, fcj, clusterName); err != nil {
			return r.failClusterKubevirt(ctx, fcj, err)
		}
		jobName := "fleet-cluster-kubevirt-" + clusterName
		var existing batchv1.Job
		if err := r.Get(ctx, client.ObjectKey{Namespace: fcj.Namespace, Name: jobName}, &existing); err == nil {
			if done, ok := jobIsFinished(&existing); done {
				if ok {
					return r.finishKubevirtJob(ctx, fcj, true, "")
				}
				_ = r.Delete(ctx, &existing, client.PropagationPolicy(metav1.DeletePropagationForeground))
			} else {
				return r.markKubevirtJobInstalling(ctx, fcj, jobName)
			}
		} else if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		extra := kubevirtJobExtraVars(fcj)
		j := jobs.ConfigureKubevirtJob(fcj.Namespace, jobName, clusterName, fcj.Spec.SiteId,
			r.Cfg.AnsibleRunnerImage, clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName), extra)
		if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, j); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return r.markKubevirtJobInstalling(ctx, fcj, jobName)
			}
			return r.failClusterKubevirt(ctx, fcj, err)
		}
		return r.markKubevirtJobInstalling(ctx, fcj, jobName)
	}

	return r.pollKubevirtJob(ctx, fcj, clusterName)
}

func kubevirtJobExtraVars(fcj *fleetv1alpha1.FleetClusterJoin) []string {
	out := []string{}
	if fcj.Spec.WantsKubevirtBridge() {
		hb := fcj.Spec.Providers.KubevirtBridge
		out = append(out,
			"-e", "fleet_cluster_kubevirt_bridge_enabled=true",
			"-e", "kubevirt_bridge_host_bridge="+hb.EffectiveBridgeName(),
			"-e", "kubevirt_bridge_nad_name=mgmt-l2",
			"-e", "kubevirt_bridge_nad_namespace=default",
		)
	} else {
		out = append(out, "-e", "fleet_cluster_kubevirt_bridge_enabled=false")
	}
	return out
}

func (r *ClusterJoinReconciler) markKubevirtJobInstalling(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, jobName string) (ctrl.Result, error) {
	key := client.ObjectKeyFromObject(fcj)
	clusterName := fcj.Spec.EffectiveClusterName(fcj.Name)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		if latest.Annotations == nil {
			latest.Annotations = map[string]string{}
		}
		latest.Annotations[configureKubevirtJobAnnotation] = jobName
		if err := r.Update(ctx, &latest); err != nil {
			return err
		}
		ensureProviderStatusDefaults(&latest, clusterName)
		st := latest.Status.Providers[fleetv1alpha1.ProviderKindKubevirt]
		st.Phase = fleetv1alpha1.ProviderPhaseInstalling
		st.JobName = jobName
		st.Message = "Installing KubeVirt platform (Multus/CDI/KubeVirt)"
		st.ProviderName = providerNameForKind(&latest, clusterName, fleetv1alpha1.ProviderKindKubevirt)
		latest.Status.Providers[fleetv1alpha1.ProviderKindKubevirt] = st
		latest.Status.Message = st.Message
		return r.Status().Update(ctx, &latest)
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *ClusterJoinReconciler) pollKubevirtJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	jobName := ""
	if st, ok := fcj.Status.Providers[fleetv1alpha1.ProviderKindKubevirt]; ok {
		jobName = st.JobName
	}
	if jobName == "" && fcj.Annotations != nil {
		jobName = fcj.Annotations[configureKubevirtJobAnnotation]
	}
	if jobName == "" {
		return ctrl.Result{}, nil
	}
	var job batchv1.Job
	if err := r.Get(ctx, client.ObjectKey{Namespace: fcj.Namespace, Name: jobName}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			if fcj.Annotations != nil && fcj.Annotations[configureKubevirtAnnotation] != "" {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			return r.finishKubevirtJob(ctx, fcj, false, "KubeVirt install job missing; retry from Console or CLI")
		}
		return ctrl.Result{}, err
	}
	done, ok, msg := r.jobDone(ctx, fcj.Namespace, jobName)
	if !done {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	if !ok {
		return r.finishKubevirtJob(ctx, fcj, false, "KubeVirt install failed: "+msg)
	}
	return r.finishKubevirtJob(ctx, fcj, true, "")
}

func (r *ClusterJoinReconciler) finishKubevirtJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, ok bool, message string) (ctrl.Result, error) {
	key := client.ObjectKeyFromObject(fcj)
	var latest fleetv1alpha1.FleetClusterJoin
	if err := r.Get(ctx, key, &latest); err != nil {
		return ctrl.Result{}, err
	}
	clusterName := latest.Spec.EffectiveClusterName(latest.Name)
	ensureProviderStatusDefaults(&latest, clusterName)

	jobName := latest.Status.Providers[fleetv1alpha1.ProviderKindKubevirt].JobName
	if jobName == "" && latest.Annotations != nil {
		jobName = latest.Annotations[configureKubevirtJobAnnotation]
	}
	logTail := r.captureNodeOpLogTail(ctx, latest.Namespace, jobName)
	finishedAt := metav1.Now()
	opPhase := fleetv1alpha1.NodeOpPhaseFailed
	if ok {
		opPhase = fleetv1alpha1.NodeOpPhaseSucceeded
	}
	if message == "" && ok {
		message = "KubeVirt platform installed"
		if latest.Spec.WantsKubevirtBridge() {
			message = "KubeVirt platform installed (LAN Multus NAD)"
		}
	}

	st := latest.Status.Providers[fleetv1alpha1.ProviderKindKubevirt]
	st.JobName = ""
	st.ProviderName = providerNameForKind(&latest, clusterName, fleetv1alpha1.ProviderKindKubevirt)
	if ok {
		st.Phase = fleetv1alpha1.ProviderPhaseReady
		st.Message = message
		latest.Status.Message = message
	} else {
		st.Phase = fleetv1alpha1.ProviderPhaseFailed
		st.Message = message
		latest.Status.Message = message
	}
	latest.Status.Providers[fleetv1alpha1.ProviderKindKubevirt] = st
	latest.Status.NodeOperationHistory = prependNodeOpHistory(latest.Status.NodeOperationHistory, fleetv1alpha1.ClusterNodeOperationRecord{
		ID:         fmt.Sprintf("kop-%d", finishedAt.UnixNano()),
		Type:       fleetv1alpha1.NodeOpTypeConfigureKubevirt,
		Node:       "kubevirt",
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
	delete(latest.Annotations, configureKubevirtAnnotation)
	delete(latest.Annotations, configureKubevirtJobAnnotation)
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

func (r *ClusterJoinReconciler) failClusterKubevirt(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, err error) (ctrl.Result, error) {
	res, finishErr := r.finishKubevirtJob(ctx, fcj, false, fmt.Sprintf("KubeVirt install failed: %v", err))
	if finishErr != nil {
		return res, finishErr
	}
	return res, err
}

// providerStatusPhase is duplicated locally to avoid exporting from types package helpers used only in tests.
func providerStatusPhase(st fleetv1alpha1.ClusterProviderStatus) string {
	if strings.TrimSpace(st.Phase) == "" {
		return fleetv1alpha1.ProviderPhaseNotInstalled
	}
	return st.Phase
}
