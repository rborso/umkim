package resourcemetrics

import (
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Utilization is cluster-level CPU/memory utilization derived from node allocatable and usage.
type Utilization struct {
	MetricsOK    bool
	CapacityOK   bool
	CPUUtil      int
	MemoryUtil   int
	MetricsAt    time.Time
	UsageMissing bool

	AllocCPUMilli       int64
	AllocMemBytes       int64
	UseCPUMilli         int64
	UseMemBytes         int64
	ReqCPUMilli         int64
	ReqMemBytes         int64
	MaxNodeCPUMilli     int64
	MaxNodeMemBytes     int64
	MaxNodeAvailableCPUMilli int64
	MaxNodeAvailableMemBytes int64
	AllocGPU                 int64
	ReqGPU                   int64
}

// AvailableCPUMilli returns allocatable minus allocated pod requests (scheduler view).
func (u Utilization) AvailableCPUMilli() int64 {
	if !u.CapacityOK {
		return 0
	}
	avail := u.AllocCPUMilli - u.ReqCPUMilli
	if avail < 0 {
		return 0
	}
	return avail
}

// AvailableMemBytes returns allocatable minus allocated pod memory requests.
func (u Utilization) AvailableMemBytes() int64 {
	if !u.CapacityOK {
		return 0
	}
	avail := u.AllocMemBytes - u.ReqMemBytes
	if avail < 0 {
		return 0
	}
	return avail
}

// AggregateCluster computes utilization % from ready nodes only.
func AggregateCluster(nodes []corev1.Node, usageByName, requestedByName map[string]corev1.ResourceList, unscheduledReq corev1.ResourceList) Utilization {
	now := time.Now().UTC()
	var allocCPU, allocMem, useCPU, useMem, reqCPU, reqMem, allocGPU, reqGPU int64
	var maxNodeCPU, maxNodeMem int64
	var maxNodeAvailCPU, maxNodeAvailMem int64
	readyCount := 0
	usageNodes := 0

	for _, n := range nodes {
		if !nodeReady(n) {
			continue
		}
		readyCount++
		nodeCPU := n.Status.Allocatable.Cpu().MilliValue()
		nodeMem := n.Status.Allocatable.Memory().Value()
		if q, ok := n.Status.Allocatable[corev1.ResourceName("nvidia.com/gpu")]; ok {
			allocGPU += q.Value()
		}
		allocCPU += nodeCPU
		allocMem += nodeMem
		if nodeCPU > maxNodeCPU {
			maxNodeCPU = nodeCPU
		}
		if nodeMem > maxNodeMem {
			maxNodeMem = nodeMem
		}

		var nodeReqCPU, nodeReqMem int64
		if r, ok := requestedByName[n.Name]; ok {
			if q, ok := r[corev1.ResourceCPU]; ok {
				nodeReqCPU = q.MilliValue()
			}
			if q, ok := r[corev1.ResourceMemory]; ok {
				nodeReqMem = q.Value()
			}
			if q, ok := r[corev1.ResourceName("nvidia.com/gpu")]; ok {
				reqGPU += q.Value()
			}
		}
		reqCPU += nodeReqCPU
		reqMem += nodeReqMem

		nodeAvailCPU := nodeCPU - nodeReqCPU
		if nodeAvailCPU < 0 {
			nodeAvailCPU = 0
		}
		nodeAvailMem := nodeMem - nodeReqMem
		if nodeAvailMem < 0 {
			nodeAvailMem = 0
		}
		if nodeAvailCPU > maxNodeAvailCPU {
			maxNodeAvailCPU = nodeAvailCPU
		}
		if nodeAvailMem > maxNodeAvailMem {
			maxNodeAvailMem = nodeAvailMem
		}

		u, ok := usageByName[n.Name]
		if !ok {
			continue
		}
		usageNodes++
		if q, ok := u[corev1.ResourceCPU]; ok {
			useCPU += q.MilliValue()
		}
		if q, ok := u[corev1.ResourceMemory]; ok {
			useMem += q.Value()
		}
	}

	if unscheduledReq != nil {
		if q, ok := unscheduledReq[corev1.ResourceCPU]; ok {
			reqCPU += q.MilliValue()
		}
		if q, ok := unscheduledReq[corev1.ResourceMemory]; ok {
			reqMem += q.Value()
		}
	}

	if readyCount == 0 || allocCPU <= 0 || allocMem <= 0 {
		return Utilization{MetricsOK: false, CapacityOK: false, MetricsAt: now, UsageMissing: true}
	}

	capacity := Utilization{
		CapacityOK:               true,
		MetricsAt:                now,
		AllocCPUMilli:            allocCPU,
		AllocMemBytes:            allocMem,
		ReqCPUMilli:              reqCPU,
		ReqMemBytes:              reqMem,
		MaxNodeCPUMilli:          maxNodeCPU,
		MaxNodeMemBytes:          maxNodeMem,
		MaxNodeAvailableCPUMilli: maxNodeAvailCPU,
		MaxNodeAvailableMemBytes: maxNodeAvailMem,
		AllocGPU:                 allocGPU,
		ReqGPU:                   reqGPU,
	}
	if usageNodes == 0 {
		capacity.MetricsOK = false
		capacity.UsageMissing = true
		return capacity
	}

	cpuPct := int(math.Round(float64(useCPU) * 100 / float64(allocCPU)))
	memPct := int(math.Round(float64(useMem) * 100 / float64(allocMem)))
	if cpuPct < 0 {
		cpuPct = 0
	}
	if memPct < 0 {
		memPct = 0
	}
	if cpuPct > 100 {
		cpuPct = 100
	}
	if memPct > 100 {
		memPct = 100
	}

	capacity.MetricsOK = true
	capacity.CPUUtil = cpuPct
	capacity.MemoryUtil = memPct
	capacity.UseCPUMilli = useCPU
	capacity.UseMemBytes = useMem
	return capacity
}

func nodeReady(n corev1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// LabelsForUtilization returns ManagedCluster labels to merge for a scrape result.
func LabelsForUtilization(u Utilization) map[string]string {
	out := map[string]string{
		LabelMetricsAt: formatMetricsAt(u.MetricsAt),
	}
	if u.CapacityOK {
		out[LabelCapacityOK] = "true"
		out[LabelCPUAllocatableMilli] = int64Str(u.AllocCPUMilli)
		out[LabelMemoryAllocatableBytes] = int64Str(u.AllocMemBytes)
		out[LabelCPUMaxNodeAllocMilli] = int64Str(u.MaxNodeCPUMilli)
		out[LabelMemoryMaxNodeAllocBytes] = int64Str(u.MaxNodeMemBytes)
		out[LabelCPURequestedMilli] = int64Str(u.ReqCPUMilli)
		out[LabelMemoryRequestedBytes] = int64Str(u.ReqMemBytes)
		out[LabelCPUMaxNodeAvailableMilli] = int64Str(u.MaxNodeAvailableCPUMilli)
		out[LabelMemoryMaxNodeAvailableBytes] = int64Str(u.MaxNodeAvailableMemBytes)
		out[LabelGPUTotal] = int64Str(u.AllocGPU)
		out[LabelGPURequested] = int64Str(u.ReqGPU)
		gpuAvail := u.AllocGPU - u.ReqGPU
		if gpuAvail < 0 {
			gpuAvail = 0
		}
		out[LabelGPUAvailable] = int64Str(gpuAvail)
	} else {
		out[LabelCapacityOK] = "false"
	}
	if !u.MetricsOK {
		out[LabelMetricsOK] = "false"
		out[LabelCPUUtilization] = ""
		out[LabelMemoryUtilization] = ""
		out[LabelCPUUsageMilli] = ""
		out[LabelMemoryUsageBytes] = ""
		return out
	}
	out[LabelMetricsOK] = "true"
	out[LabelCPUUtilization] = intStr(u.CPUUtil)
	out[LabelMemoryUtilization] = intStr(u.MemoryUtil)
	out[LabelCPUUsageMilli] = int64Str(u.UseCPUMilli)
	out[LabelMemoryUsageBytes] = int64Str(u.UseMemBytes)
	return out
}

func int64Str(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [24]byte
	i := len(buf)
	v := n
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func intStr(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	v := n
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
