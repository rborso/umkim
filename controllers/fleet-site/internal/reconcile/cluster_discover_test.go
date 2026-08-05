package reconcile

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestJobIsFinishedDiscoverComplete(t *testing.T) {
	job := &batchv1.Job{
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{{
				Type:   batchv1.JobComplete,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	done, ok := jobIsFinished(job)
	if !done || !ok {
		t.Fatalf("expected completed discover job to be finished ok=true, got done=%v ok=%v", done, ok)
	}
}

func TestJobIsFinishedDiscoverActive(t *testing.T) {
	active := int32(1)
	job := &batchv1.Job{Status: batchv1.JobStatus{Active: active}}
	done, _ := jobIsFinished(job)
	if done {
		t.Fatal("expected active job to not be finished")
	}
}
