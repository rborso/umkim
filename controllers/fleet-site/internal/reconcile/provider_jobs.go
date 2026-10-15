package reconcile

import (
	"context"
	"errors"
	"fmt"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var errJobTerminating = errors.New("provider job is terminating")

// ensureProviderJob creates the Ansible Job, replacing a finished predecessor and
// waiting for a terminating one. Status must only be updated after this returns nil.
func (r *Reconciler) ensureProviderJob(ctx context.Context, j *batchv1.Job) error {
	key := types.NamespacedName{Namespace: j.Namespace, Name: j.Name}
	var existing batchv1.Job
	err := r.Get(ctx, key, &existing)
	if err == nil {
		if existing.DeletionTimestamp != nil {
			return errJobTerminating
		}
		if done, _ := jobIsFinished(&existing); done {
			if derr := r.Delete(ctx, &existing); derr != nil && !apierrors.IsNotFound(derr) {
				return fmt.Errorf("delete finished job %s: %w", j.Name, derr)
			}
			if werr := r.waitJobAbsent(ctx, key, 2*time.Minute); werr != nil {
				return werr
			}
			// The Job name is reused, so its pods must go with it. Background
			// deletion leaves them orphaned when the garbage collector has not
			// caught up, and an orphaned terminated pod is only reclaimed by the
			// pod GC's high-water mark — they accumulate on the hub otherwise.
			if perr := r.deleteJobPods(ctx, j.Namespace, j.Name); perr != nil {
				return perr
			}
		} else {
			return nil
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	if err := r.Create(ctx, j); err != nil {
		return err
	}
	return nil
}

// deleteJobPods removes the pods a Job left behind. The Job is gone by the time
// this runs, so they are orphans: background deletion relies on the garbage
// collector, and the pod GC only reclaims orphaned terminated pods past a high
// water mark. Best effort — a pod that is already gone is not an error.
func (r *Reconciler) deleteJobPods(ctx context.Context, ns, jobName string) error {
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(ns), client.MatchingLabels{"job-name": jobName}); err != nil {
		return fmt.Errorf("list pods of job %s: %w", jobName, err)
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if err := r.Delete(ctx, pod, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete pod %s of job %s: %w", pod.Name, jobName, err)
		}
	}
	return nil
}

func (r *Reconciler) waitJobAbsent(ctx context.Context, key types.NamespacedName, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var job batchv1.Job
		err := r.Get(ctx, key, &job)
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out waiting for job %s deletion", key.Name)
}

// recoverOrphanedProviderStatuses resets provider rows whose Jobs are missing.
// Retained for status hygiene helpers/tests; legacy service-provider install Jobs are gone.
func recoverOrphanedProviderStatuses(providers map[string]fleetv1alpha1.ClusterProviderStatus, orphanKinds map[string]struct{}) (map[string]fleetv1alpha1.ClusterProviderStatus, bool) {
	if len(orphanKinds) == 0 {
		return providers, false
	}
	changed := false
	out := make(map[string]fleetv1alpha1.ClusterProviderStatus, len(providers))
	for kind, st := range providers {
		out[kind] = st
		if _, orphan := orphanKinds[kind]; !orphan {
			continue
		}
		st.JobName = ""
		if st.Phase == fleetv1alpha1.ProviderPhaseDeleting {
			st.Phase = fleetv1alpha1.ProviderPhaseNotInstalled
			st.Message = "Provider removed"
		} else {
			st.Phase = fleetv1alpha1.ProviderPhaseNotInstalled
			st.Message = "Provider install job missing; retrying"
		}
		out[kind] = st
		changed = true
	}
	return out, changed
}

func isJobTerminating(err error) bool {
	return errors.Is(err, errJobTerminating)
}
