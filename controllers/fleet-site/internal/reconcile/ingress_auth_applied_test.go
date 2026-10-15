package reconcile

import (
	"testing"

	"github.com/rborso/umkim/controllers/fleet-site/internal/ingressauth"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAppliedClusters(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{jobs.AppliedClustersAnnotation: "cluster-a,cluster-b"},
		},
	}
	got := appliedClusters(job)
	if len(got) != 2 || got[0] != "cluster-a" || got[1] != "cluster-b" {
		t.Fatalf("appliedClusters = %v", got)
	}
	// No annotation (a Job from before the annotation existed) means "unknown",
	// and the caller keeps the previous behaviour.
	if appliedClusters(&batchv1.Job{}) != nil {
		t.Fatal("missing annotation should report unknown (nil)")
	}
}

func TestEnabledSubsetRestrictsAndSorts(t *testing.T) {
	desired := ingressauth.DesiredState{EnabledClusters: []string{"cluster-c", "cluster-a", "cluster-b"}}
	got := enabledSubset(desired, []string{"cluster-b", "cluster-a"})
	if len(got.EnabledClusters) != 2 || got.EnabledClusters[0] != "cluster-a" || got.EnabledClusters[1] != "cluster-b" {
		t.Fatalf("enabledSubset = %v", got.EnabledClusters)
	}
	// The source state must not be mutated.
	if len(desired.EnabledClusters) != 3 {
		t.Fatalf("source state mutated: %v", desired.EnabledClusters)
	}
}
