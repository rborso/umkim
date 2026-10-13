package yaidap

import (
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

// ValidateActionForKind checks whether a provision action is allowed for the host kind.
func ValidateActionForKind(action, kind string) error {
	action = strings.TrimSpace(action)
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return fmt.Errorf("host kind is required")
	}
	allowed := actionAllowedForKind(action, kind)
	if !allowed {
		return fmt.Errorf("action %q is not allowed for host kind %q", action, kind)
	}
	return nil
}

func actionAllowedForKind(action, kind string) bool {
	switch action {
	case fleetv1alpha1.ProvisionActionCreateVM,
		fleetv1alpha1.ProvisionActionPlanVM,
		fleetv1alpha1.ProvisionActionDestroy:
		return kind == fleetv1alpha1.SiteHostKindVM
	case fleetv1alpha1.ProvisionActionDiscover:
		return kind == fleetv1alpha1.SiteHostKindHypervisor || kind == fleetv1alpha1.SiteHostKindBareMetal
	case fleetv1alpha1.ProvisionActionDiscoverPXE:
		return kind == fleetv1alpha1.SiteHostKindHypervisor || kind == fleetv1alpha1.SiteHostKindBareMetal
	case fleetv1alpha1.ProvisionActionInstallBM:
		return kind == fleetv1alpha1.SiteHostKindHypervisor || kind == fleetv1alpha1.SiteHostKindBareMetal
	case fleetv1alpha1.ProvisionActionStatus,
		fleetv1alpha1.ProvisionActionVerify:
		return kind == fleetv1alpha1.SiteHostKindHypervisor || kind == fleetv1alpha1.SiteHostKindBareMetal
	case fleetv1alpha1.ProvisionActionPrepareHypervisor,
		fleetv1alpha1.ProvisionActionPrepareGPUPassthrough,
		fleetv1alpha1.ProvisionActionPrepareHypervisorGPU:
		return kind == fleetv1alpha1.SiteHostKindHypervisor
	default:
		return true
	}
}
