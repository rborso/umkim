package capdiscover

import (
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// NodeGPUCount returns allocatable GPU count from node capacity (prefers nvidia.com/gpu).
func NodeGPUCount(capacity corev1.ResourceList) int {
	if len(capacity) == 0 {
		return 0
	}
	if q, ok := capacity[corev1.ResourceName("nvidia.com/gpu")]; ok {
		return int(q.Value())
	}
	total := 0
	for k, q := range capacity {
		if strings.Contains(strings.ToLower(string(k)), "gpu") {
			total += int(q.Value())
		}
	}
	return total
}

// ApplyGPUResourceLabels sets fleet.resource/gpu.* on ManagedCluster labels from discovery.
func ApplyGPUResourceLabels(labels map[string]string, total, available int) {
	if labels == nil {
		return
	}
	if total < 0 {
		total = 0
	}
	if available < 0 {
		available = 0
	}
	labels["fleet.resource/gpu.total"] = strconv.Itoa(total)
	labels["fleet.resource/gpu.available"] = strconv.Itoa(available)
}
