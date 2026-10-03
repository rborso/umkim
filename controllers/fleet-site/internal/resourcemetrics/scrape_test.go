package resourcemetrics

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListNodeRequested_unscheduledPods(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("100m"),
					},
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	})
	_, unscheduled, err := (&Scraper{}).listNodeRequested(context.Background(), cs)
	if err != nil {
		t.Fatal(err)
	}
	if unscheduled == nil {
		t.Fatal("expected initialized unscheduled resource list")
	}
	if unscheduled.Cpu().MilliValue() != 100 {
		t.Fatalf("unscheduled cpu: got %dm want 100m", unscheduled.Cpu().MilliValue())
	}
}
