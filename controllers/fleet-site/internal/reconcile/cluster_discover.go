package reconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/capdiscover"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/registry"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const discoverAnnotation = "fleet.umkim.com/discover-capabilities"

type discoverPayload struct {
	Nodes               []capdiscover.NodeResult `json:"nodes"`
	ClusterCapabilities map[string]bool          `json:"cluster_capabilities"`
	ClusterResources    capdiscover.IntMap       `json:"cluster_resources,omitempty"`
}

func (r *ClusterJoinReconciler) reconcileCapabilityDiscover(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	if fcj.Annotations == nil {
		return ctrl.Result{}, nil
	}
	if _, ok := fcj.Annotations[discoverAnnotation]; !ok {
		return ctrl.Result{}, nil
	}

	jobAnn := fcj.Annotations["fleet.umkim.com/capability-discover-job"]
	if jobAnn == "" {
		if err := capdiscover.Begin(ctx, r.Client, clusterName); err != nil && err != capdiscover.ErrAlreadyRunning {
			return ctrl.Result{}, err
		}
		nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
		if err != nil {
			_ = capdiscover.Fail(ctx, r.Client, clusterName, err.Error())
			return ctrl.Result{}, err
		}
		if err := r.ensureClusterInventoryCM(ctx, fcj, clusterName, nodeWG); err != nil {
			_ = capdiscover.Fail(ctx, r.Client, clusterName, err.Error())
			return ctrl.Result{}, err
		}
		if _, err := r.ensureClusterSSHSecret(ctx, fcj, clusterName); err != nil {
			_ = capdiscover.Fail(ctx, r.Client, clusterName, err.Error())
			return ctrl.Result{}, err
		}
		jobName := "fleet-cluster-discover-" + clusterName
		jobKey := types.NamespacedName{Name: jobName, Namespace: fcj.Namespace}
		var existing batchv1.Job
		getErr := r.Get(ctx, jobKey, &existing)
		switch {
		case apierrors.IsNotFound(getErr):
			if err := r.createDiscoverJob(ctx, fcj, clusterName, jobName); err != nil {
				_ = capdiscover.Fail(ctx, r.Client, clusterName, err.Error())
				return ctrl.Result{}, err
			}
		case getErr != nil:
			return ctrl.Result{}, getErr
		default:
			if done, _ := jobIsFinished(&existing); done {
				if err := r.resetDiscoverArtifacts(ctx, fcj.Namespace, clusterName, jobName); err != nil {
					return ctrl.Result{}, err
				}
				if err := r.createDiscoverJob(ctx, fcj, clusterName, jobName); err != nil {
					_ = capdiscover.Fail(ctx, r.Client, clusterName, err.Error())
					return ctrl.Result{}, err
				}
			}
		}
		if fcj.Annotations == nil {
			fcj.Annotations = map[string]string{}
		}
		fcj.Annotations["fleet.umkim.com/capability-discover-job"] = jobName
		if err := r.Update(ctx, fcj); err != nil {
			return ctrl.Result{}, err
		}
		fcj.Status.Message = "Capability discovery running"
		if err := r.Status().Update(ctx, fcj); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Use jobPollState so a missing Job is terminal (GC/expiry); jobDone waits forever on NotFound.
	done, ok, msg := r.jobPollState(ctx, fcj.Namespace, jobAnn, false)
	if !done {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	delete(fcj.Annotations, discoverAnnotation)
	delete(fcj.Annotations, "fleet.umkim.com/capability-discover-job")
	_ = r.Update(ctx, fcj)

	if !ok {
		_ = capdiscover.Fail(ctx, r.Client, clusterName, "discovery job failed: "+msg)
		fcj.Status.Message = "Capability discovery failed"
		_ = r.Status().Update(ctx, fcj)
		return ctrl.Result{}, nil
	}

	payload, err := r.readDiscoverConfigMap(ctx, fcj.Namespace, clusterName)
	if err != nil {
		_ = capdiscover.Fail(ctx, r.Client, clusterName, err.Error())
		fcj.Status.Message = "Capability discovery failed"
		_ = r.Status().Update(ctx, fcj)
		return ctrl.Result{}, err
	}
	if err := r.applyDiscoverPayload(ctx, fcj, clusterName, payload); err != nil {
		_ = capdiscover.Fail(ctx, r.Client, clusterName, err.Error())
		fcj.Status.Message = "Capability discovery failed"
		_ = r.Status().Update(ctx, fcj)
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	fcj.Status.Message = "Capability discovery complete"
	_ = r.Status().Update(ctx, fcj)
	return ctrl.Result{}, nil
}

func (r *ClusterJoinReconciler) createDiscoverJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName, jobName string) error {
	j := jobs.DiscoverJob(fcj.Namespace, jobName, clusterName, fcj.Spec.SiteId,
		r.Cfg.AnsibleRunnerImage, clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName))
	j.Name = jobName
	if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, j); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (r *ClusterJoinReconciler) resetDiscoverArtifacts(ctx context.Context, ns, clusterName, jobName string) error {
	_ = r.Delete(ctx, &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: ns},
	}, client.PropagationPolicy(metav1.DeletePropagationBackground))
	return client.IgnoreNotFound(r.Delete(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fleet-cluster-discover-" + clusterName,
			Namespace: ns,
		},
	}))
}

func (r *ClusterJoinReconciler) readDiscoverConfigMap(ctx context.Context, ns, clusterName string) (*discoverPayload, error) {
	var cm corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Name: "fleet-cluster-discover-" + clusterName, Namespace: ns}, &cm); err != nil {
		return nil, fmt.Errorf("read discover configmap: %w", err)
	}
	raw := cm.Data["results.json"]
	if raw == "" {
		return nil, fmt.Errorf("discover configmap missing results.json")
	}
	var p discoverPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *ClusterJoinReconciler) applyDiscoverPayload(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string, p *discoverPayload) error {
	if p.ClusterCapabilities == nil {
		return fmt.Errorf("empty cluster capabilities in discovery results")
	}
	var mc unstructured.Unstructured
	mc.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	if err := r.Get(ctx, types.NamespacedName{Name: clusterName}, &mc); err != nil {
		return err
	}
	labels := mc.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	registry.ApplyCapabilityPatch(labels, p.ClusterCapabilities)
	if p.ClusterResources != nil {
		capdiscover.ApplyGPUResourceLabels(labels, p.ClusterResources["gpu.total"], p.ClusterResources["gpu.available"])
	}
	mc.SetLabels(labels)

	res := &capdiscover.Result{
		Phase:               capdiscover.PhaseSucceeded,
		Message:             "Capabilities discovered",
		CompletedAt:         time.Now().UTC().Format(time.RFC3339),
		Nodes:               p.Nodes,
		ClusterCapabilities: p.ClusterCapabilities,
		ClusterResources:    p.ClusterResources,
	}
	ann := mc.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	if err := capdiscover.WriteResultAnnotations(ann, res); err != nil {
		return err
	}
	mc.SetAnnotations(ann)
	if err := r.Update(ctx, &mc); err != nil {
		return err
	}

	var latest fleetv1alpha1.FleetClusterJoin
	if err := r.Get(ctx, types.NamespacedName{Namespace: fcj.Namespace, Name: fcj.Name}, &latest); err != nil {
		return err
	}
	if latest.Spec.ClusterCapabilities == nil {
		latest.Spec.ClusterCapabilities = map[string]bool{}
	}
	for k, v := range p.ClusterCapabilities {
		switch k {
		case registry.CapGPU, registry.CapStorageRWO, registry.CapStorageRWX, registry.CapKVM, registry.CapHATopology:
			latest.Spec.ClusterCapabilities[k] = v
		}
	}
	if err := r.Update(ctx, &latest); err != nil {
		return err
	}
	return nil
}
