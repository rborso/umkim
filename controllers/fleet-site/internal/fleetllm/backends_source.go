package fleetllm

import fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"

// BackendsManual reports whether FleetLLM backends were set via API/console JSON override.
func BackendsManual(fl *fleetv1alpha1.FleetLLM) bool {
	if fl == nil || fl.Annotations == nil {
		return false
	}
	return fl.Annotations[BackendsManualAnnotation] == "true"
}
