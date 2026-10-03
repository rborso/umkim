package resourcemetrics

import "time"

const (
	LabelCPUUtilization           = "fleet.resource/cpu.utilization"
	LabelMemoryUtilization        = "fleet.resource/memory.utilization"
	LabelMetricsAt                = "fleet.resource/metrics_at"
	LabelMetricsOK                = "fleet.resource/metrics_ok"
	LabelCPUAllocatableMilli      = "fleet.resource/cpu.allocatable_milli"
	LabelMemoryAllocatableBytes   = "fleet.resource/memory.allocatable_bytes"
	LabelCPUUsageMilli            = "fleet.resource/cpu.usage_milli"
	LabelMemoryUsageBytes         = "fleet.resource/memory.usage_bytes"
	LabelCPUMaxNodeAllocMilli      = "fleet.resource/cpu.max_node_allocatable_milli"
	LabelMemoryMaxNodeAllocBytes   = "fleet.resource/memory.max_node_allocatable_bytes"
	LabelCPURequestedMilli         = "fleet.resource/cpu.requested_milli"
	LabelMemoryRequestedBytes      = "fleet.resource/memory.requested_bytes"
	LabelCPUMaxNodeAvailableMilli  = "fleet.resource/cpu.max_node_available_milli"
	LabelMemoryMaxNodeAvailableBytes = "fleet.resource/memory.max_node_available_bytes"
	LabelCapacityOK                = "fleet.resource/capacity_ok"
	LabelGPUTotal                  = "fleet.resource/gpu.total"
	LabelGPURequested              = "fleet.resource/gpu.requested"
	LabelGPUAvailable              = "fleet.resource/gpu.available"
)

// metricsAtLabelFormat is label-safe (no ':' characters); control-plane accepts this and RFC3339.
const metricsAtLabelFormat = "20060102T150405Z"

func formatMetricsAt(t time.Time) string {
	return t.UTC().Format(metricsAtLabelFormat)
}

func parseMetricsAt(v string) (time.Time, bool) {
	if t, err := time.Parse(metricsAtLabelFormat, v); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	return time.Time{}, false
}
