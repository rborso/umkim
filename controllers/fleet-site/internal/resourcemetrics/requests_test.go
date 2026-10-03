package resourcemetrics

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodRequests_initMaxPlusContainers(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("500m"),
					},
				},
			}},
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			}},
		},
	}
	req := podRequests(pod)
	if req.Cpu().MilliValue() != 700 {
		t.Fatalf("cpu: got %dm want 700m", req.Cpu().MilliValue())
	}
}

func TestAggregateCluster_nodeAvailableUsesRequests(t *testing.T) {
	nodes := []corev1.Node{readyNode("a", "8", "32Gi")}
	requested := map[string]corev1.ResourceList{
		"a": {
			corev1.ResourceCPU:    resource.MustParse("926m"),
			corev1.ResourceMemory: resource.MustParse("898Mi"),
		},
	}
	usage := map[string]corev1.ResourceList{
		"a": {
			corev1.ResourceCPU:    resource.MustParse("356m"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
	u := AggregateCluster(nodes, usage, requested, nil)
	if u.MaxNodeAvailableCPUMilli != 8000-926 {
		t.Fatalf("max node avail cpu: got %d want %d", u.MaxNodeAvailableCPUMilli, 8000-926)
	}
	if u.AvailableCPUMilli() != 8000-926 {
		t.Fatalf("cluster avail cpu: got %d want %d", u.AvailableCPUMilli(), 8000-926)
	}
}
