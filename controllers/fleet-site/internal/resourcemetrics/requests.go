package resourcemetrics

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// podRequests returns scheduler-equivalent pod resource requests (init max + container sum).
func podRequests(pod *corev1.Pod) corev1.ResourceList {
	if pod == nil {
		return nil
	}
	var initCPU, initMem resource.Quantity
	for _, c := range pod.Spec.InitContainers {
		if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
			if q.Cmp(initCPU) > 0 {
				initCPU = q
			}
		}
		if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			if q.Cmp(initMem) > 0 {
				initMem = q
			}
		}
	}

	out := corev1.ResourceList{}
	for _, c := range pod.Spec.Containers {
		for name, qty := range c.Resources.Requests {
			if existing, ok := out[name]; ok {
				existing.Add(qty)
				out[name] = existing
			} else {
				out[name] = qty.DeepCopy()
			}
		}
	}
	if !initCPU.IsZero() {
		if existing, ok := out[corev1.ResourceCPU]; ok {
			existing.Add(initCPU)
			out[corev1.ResourceCPU] = existing
		} else {
			out[corev1.ResourceCPU] = initCPU.DeepCopy()
		}
	}
	if !initMem.IsZero() {
		if existing, ok := out[corev1.ResourceMemory]; ok {
			existing.Add(initMem)
			out[corev1.ResourceMemory] = existing
		} else {
			out[corev1.ResourceMemory] = initMem.DeepCopy()
		}
	}
	return out
}

func podSchedulable(pod *corev1.Pod) bool {
	switch pod.Status.Phase {
	case corev1.PodSucceeded, corev1.PodFailed:
		return false
	default:
		return true
	}
}
