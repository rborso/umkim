package siteprovision

import (
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

// ActionBusy reports whether the site already has a provision action pending or
// running. The connector runs one action per site at a time, so controller-driven
// callers (the cluster bring-up pipeline and cluster delete) must wait for this
// to clear before starting their own action.
func ActionBusy(fs *fleetv1alpha1.FleetSite) bool {
	if fs == nil || fs.Status.Provision == nil || fs.Status.Provision.Current == nil {
		return false
	}
	p := fs.Status.Provision.Current.Phase
	return p == fleetv1alpha1.ProvisionPhasePending || p == fleetv1alpha1.ProvisionPhaseRunning
}
