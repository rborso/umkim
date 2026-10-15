package reconcile

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// A Job name is reused, so replacing a finished Job must take its pods with it.
// Background deletion relies on the garbage collector, and an orphaned terminated
// pod is only reclaimed by the pod GC's high water mark — they accumulated on the
// hub as Error/Completed pods with no owner.
func TestEnsureProviderJobReplacesFinishedJobAndItsPods(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	ns := "fleet-console"
	finished := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "fa-i-app", Namespace: ns},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}},
		},
	}
	// The pod the finished Job left behind: no owner (the Job is gone by then).
	orphan := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fa-i-app-pod",
			Namespace: ns,
			Labels:    map[string]string{"job-name": "fa-i-app"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodFailed},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(finished, orphan).Build()
	r := &Reconciler{Client: c, Scheme: scheme, Cfg: config.Config{Namespace: ns}}

	replacement := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "fa-i-app", Namespace: ns}}
	if err := r.ensureProviderJob(context.Background(), replacement); err != nil {
		t.Fatalf("ensureProviderJob: %v", err)
	}

	// The replacement Job exists again.
	var job batchv1.Job
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "fa-i-app"}, &job); err != nil {
		t.Fatalf("replacement job: %v", err)
	}
	if len(job.Status.Conditions) != 0 {
		t.Fatalf("replacement job kept the old status: %+v", job.Status.Conditions)
	}
	// And the predecessor's pod is gone, not orphaned.
	var pod corev1.Pod
	err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "fa-i-app-pod"}, &pod)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("orphaned pod survived the replacement (err=%v)", err)
	}
	_ = fleetv1alpha1.ProviderKindK8s
}
