package reconcile

import (
	"context"
	"errors"
	"fmt"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
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
