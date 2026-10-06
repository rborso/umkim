package reconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/overlay"
	"github.com/rborso/umkim/controllers/fleet-site/internal/wg"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	annOverlayCheckAt       = "fleet.umkim.com/overlay-check-at"
	annOverlayCheckJob      = "fleet.umkim.com/overlay-check-job"
	annOverlayRecoverJob    = "fleet.umkim.com/overlay-recover-job"
	annOverlayReachable     = "fleet.umkim.com/overlay-reachable"
	annOverlayFailCount     = "fleet.umkim.com/overlay-fail-count"
	labelOverlayReachable   = "fleet.health/overlay_reachable"
	overlayCheckInterval    = 60 * time.Second
	overlayRecoverFailCount = 3
	// overlayAPIPort is the spoke kube-apiserver port reachable over WireGuard
	// (the same endpoint the synced spoke kubeconfig uses).
	overlayAPIPort = 6443
)

func (r *ClusterJoinReconciler) reconcileOverlayConnectivity(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	if fcj.Status.WireguardAddress == "" || fcj.Status.WireguardPublicKey == "" {
		return ctrl.Result{}, nil
	}
	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}

	if jobName := fcj.Annotations[annOverlayRecoverJob]; jobName != "" {
		done, ok, msg := r.jobDone(ctx, fcj.Namespace, jobName)
		if !done {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		delete(fcj.Annotations, annOverlayRecoverJob)
		if ok {
			fcj.Annotations[annOverlayFailCount] = "0"
			fcj.Annotations[annOverlayReachable] = "true"
			fcj.Status.Message = "Overlay reachable; provider health refreshing"
		} else {
			fcj.Status.Message = fmt.Sprintf("Overlay recovery failed: %s", msg)
		}
		_ = r.patchOverlayReachableLabel(ctx, clusterName, ok)
		_ = r.Update(ctx, fcj)
		_ = r.Status().Update(ctx, fcj)
		return ctrl.Result{RequeueAfter: overlayCheckInterval}, nil
	}

	if jobName := fcj.Annotations[annOverlayCheckJob]; jobName != "" {
		done, ok, msg := r.jobDone(ctx, fcj.Namespace, jobName)
		if !done {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		delete(fcj.Annotations, annOverlayCheckJob)
		fcj.Annotations[annOverlayCheckAt] = strconv.FormatInt(time.Now().Unix(), 10)
		if ok {
			fcj.Annotations[annOverlayFailCount] = "0"
			wasDown := fcj.Annotations[annOverlayReachable] == "false"
			fcj.Annotations[annOverlayReachable] = "true"
			if wasDown {
				if err := r.startOverlayRecover(ctx, fcj, clusterName); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}
			fcj.Status.Message = "Cluster ready"
		} else {
			fails := overlayFailCount(fcj.Annotations[annOverlayFailCount]) + 1
			fcj.Annotations[annOverlayFailCount] = strconv.Itoa(fails)
			fcj.Annotations[annOverlayReachable] = "false"
			fcj.Status.Message = fmt.Sprintf("Hub overlay unreachable (%s); tunnel recovery in progress", msg)
			if fails >= overlayRecoverFailCount {
				if err := r.startOverlayRecover(ctx, fcj, clusterName); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}
		}
		_ = r.patchOverlayReachableLabel(ctx, clusterName, ok)
		_ = r.Update(ctx, fcj)
		_ = r.Status().Update(ctx, fcj)
		return ctrl.Result{RequeueAfter: overlayCheckInterval}, nil
	}

	if last := fcj.Annotations[annOverlayCheckAt]; last != "" {
		if ts, err := strconv.ParseInt(last, 10, 64); err == nil {
			if time.Since(time.Unix(ts, 0)) < overlayCheckInterval {
				return ctrl.Result{RequeueAfter: overlayCheckInterval}, nil
			}
		}
	}

	hostIP := wg.HostIP(fcj.Status.WireguardAddress)
	// Probe the endpoint the hub actually uses to reach the spoke: the
	// kube-apiserver on 6443 over WireGuard. spec.providers.k8sNodePort is
	// allocated for bookkeeping but nothing listens on it, so probing it caused
	// false "Hub overlay unreachable" alarms and repeated recover jobs.
	tcpPort := overlayAPIPort
	jobName := fmt.Sprintf("fleet-overlay-check-%s-%d", clusterName, time.Now().Unix())
	if err := r.pruneOverlayCheckJobs(ctx, fcj.Namespace, clusterName); err != nil {
		return ctrl.Result{}, err
	}
	j := jobs.OverlayConnectivityJob(fcj.Namespace, jobName, fcj.Status.WireguardPublicKey, hostIP, tcpPort)
	if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Create(ctx, j); err != nil && !errors.IsAlreadyExists(err) {
		return ctrl.Result{}, err
	}
	fcj.Annotations[annOverlayCheckJob] = jobName
	if err := r.Update(ctx, fcj); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// pruneOverlayCheckJobs removes finished overlay probe jobs for a cluster so periodic
// checks do not accumulate hundreds of completed pods in fleet-console.
func (r *ClusterJoinReconciler) pruneOverlayCheckJobs(ctx context.Context, ns, clusterName string) error {
	prefix := fmt.Sprintf("fleet-overlay-check-%s-", clusterName)
	var list batchv1.JobList
	if err := r.List(ctx, &list, client.InNamespace(ns), client.MatchingLabels{"app": "fleet-site-overlay-check"}); err != nil {
		return err
	}
	for i := range list.Items {
		job := &list.Items[i]
		if !strings.HasPrefix(job.Name, prefix) {
			continue
		}
		if job.Status.Active > 0 {
			continue
		}
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func overlayFailCount(raw string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(raw))
	return n
}

func (r *ClusterJoinReconciler) resyncHubWGPeer(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) error {
	nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
	if err != nil {
		return err
	}
	peers := make([]jobs.WGPeer, 0, len(nodeWG))
	for _, n := range nodeWG {
		peers = append(peers, jobs.WGPeer{PublicKey: n.PublicKey, AllowedIP: wg.HostIP(n.WGAddress)})
	}
	if len(peers) == 0 {
		return nil
	}
	return r.ensureClusterWgSyncJob(ctx, fcj, clusterName, peers, nil)
}

func (r *ClusterJoinReconciler) startOverlayRecover(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) error {
	if err := r.applySpokeWGRestartManifestWork(ctx, fcj, clusterName); err != nil {
		return err
	}
	nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
	if err != nil {
		return err
	}
	if err := r.ensureClusterInventoryCM(ctx, fcj, clusterName, nodeWG); err != nil {
		return err
	}
	if _, err := r.ensureClusterSSHSecret(ctx, fcj, clusterName); err != nil {
		return err
	}
	jobName := fmt.Sprintf("fleet-overlay-recover-%s-%d", clusterName, time.Now().Unix())
	extra := []string{
		"-e", "fleet_site_wg_public_key=" + fcj.Status.WireguardPublicKey,
		"-e", "fleet_site_wg_address=" + fcj.Status.WireguardAddress,
	}
	j := jobs.OverlayRecoverJob(fcj.Namespace, jobName, clusterName, fcj.Spec.SiteId,
		r.Cfg.AnsibleRunnerImage, clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName), extra)
	if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, j); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	fcj.Annotations[annOverlayRecoverJob] = jobName
	return r.Update(ctx, fcj)
}

func (r *ClusterJoinReconciler) patchOverlayReachableLabel(ctx context.Context, clusterName string, reachable bool) error {
	var mc unstructured.Unstructured
	mc.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	if err := r.Get(ctx, types.NamespacedName{Name: clusterName}, &mc); err != nil {
		return client.IgnoreNotFound(err)
	}
	labels, _, _ := unstructured.NestedStringMap(mc.Object, "metadata", "labels")
	if labels == nil {
		labels = map[string]string{}
	}
	val := "false"
	if reachable {
		val = "true"
	}
	if labels[labelOverlayReachable] == val {
		return nil
	}
	labels[labelOverlayReachable] = val
	_ = unstructured.SetNestedStringMap(mc.Object, labels, "metadata", "labels")
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]string{labelOverlayReachable: val},
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	return r.Patch(ctx, &mc, client.RawPatch(types.MergePatchType, patchBytes))
}

func (r *ClusterJoinReconciler) applySpokeWGRestartManifestWork(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) error {
	_ = fcj
	if err := r.pruneSpokeWGRestartManifestWorks(ctx, clusterName); err != nil {
		return err
	}
	workName := overlay.SpokeWGRestartWorkName(clusterName)
	mw := overlay.SpokeWGRestartManifestWork(clusterName, workName)
	if err := r.Create(ctx, mw); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// pruneSpokeWGRestartManifestWorks drops earlier overlay-restart works so a
// disconnected work-agent cannot replay a burst of wg-quick restarts.
func (r *ClusterJoinReconciler) pruneSpokeWGRestartManifestWorks(ctx context.Context, clusterName string) error {
	var list unstructured.UnstructuredList
	list.SetAPIVersion("work.open-cluster-management.io/v1")
	list.SetKind("ManifestWork")
	if err := r.List(ctx, &list, client.InNamespace(clusterName), client.MatchingLabels{
		"fleet.umkim.com/overlay-recover": "true",
	}); err != nil {
		return client.IgnoreNotFound(err)
	}
	for i := range list.Items {
		mw := &list.Items[i]
		if err := r.Delete(ctx, mw); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
