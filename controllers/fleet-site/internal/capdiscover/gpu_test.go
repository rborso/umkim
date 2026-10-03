package capdiscover

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestNodeGPUCount(t *testing.T) {
	cap := corev1.ResourceList{
		corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("4"),
	}
	if got := NodeGPUCount(cap); got != 4 {
		t.Fatalf("expected 4, got %d", got)
	}
}

func TestApplyGPUResourceLabels(t *testing.T) {
	labels := map[string]string{}
	ApplyGPUResourceLabels(labels, 3, 2)
	if labels["fleet.resource/gpu.total"] != "3" || labels["fleet.resource/gpu.available"] != "2" {
		t.Fatalf("unexpected labels: %v", labels)
	}
}
