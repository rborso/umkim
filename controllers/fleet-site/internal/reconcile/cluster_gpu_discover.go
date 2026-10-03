package reconcile

import (
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

// gpuCapabilityDiscoverDeferred returns true when capability discovery should wait
// until GPU runtime enable finishes so fleet.capability/gpu reflects device plugin capacity.
func gpuCapabilityDiscoverDeferred(fcj *fleetv1alpha1.FleetClusterJoin) bool {
	if fcj == nil {
		return false
	}
	if fcj.Status.Gpu.RuntimePhase == fleetv1alpha1.GpuRuntimePhaseApplying {
		return true
	}
	if fcj.Status.Gpu.RuntimeJobName != "" {
		return true
	}
	if fcj.Annotations != nil {
		if _, ok := fcj.Annotations[configureGpuEnableAnnotation]; ok {
			return true
		}
	}
	// Host driver prepared but container runtime / device plugin not enabled yet.
	if fcj.Status.Gpu.HostPhase == fleetv1alpha1.GpuHostPhaseReady &&
		fcj.Status.Gpu.RuntimePhase != fleetv1alpha1.GpuRuntimePhaseReady &&
		fcj.Status.Gpu.RuntimePhase != fleetv1alpha1.GpuRuntimePhaseFailed {
		return true
	}
	return false
}
