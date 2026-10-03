package reconcile

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestGpuCapabilityDiscoverDeferred_hostReadyRuntimePending(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			Gpu: fleetv1alpha1.ClusterGpuStatus{
				HostPhase:    fleetv1alpha1.GpuHostPhaseReady,
				RuntimePhase: fleetv1alpha1.GpuRuntimePhaseNotEnabled,
			},
		},
	}
	if !gpuCapabilityDiscoverDeferred(fcj) {
		t.Fatal("expected defer when host ready and runtime not enabled")
	}
}

func TestGpuCapabilityDiscoverDeferred_runtimeReady(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			Gpu: fleetv1alpha1.ClusterGpuStatus{
				HostPhase:    fleetv1alpha1.GpuHostPhaseReady,
				RuntimePhase: fleetv1alpha1.GpuRuntimePhaseReady,
			},
		},
	}
	if gpuCapabilityDiscoverDeferred(fcj) {
		t.Fatal("expected no defer when runtime is ready")
	}
}

func TestGpuCapabilityDiscoverDeferred_enableApplying(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			Gpu: fleetv1alpha1.ClusterGpuStatus{
				RuntimePhase: fleetv1alpha1.GpuRuntimePhaseApplying,
			},
		},
	}
	if !gpuCapabilityDiscoverDeferred(fcj) {
		t.Fatal("expected defer while runtime enable is applying")
	}
}
