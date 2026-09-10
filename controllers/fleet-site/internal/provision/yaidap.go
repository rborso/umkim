package provision

import (
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

// ValidateProvider returns an error if provider is unknown.
func ValidateProvider(provider string) error {
	switch strings.TrimSpace(provider) {
	case "", fleetv1alpha1.ProvisionProviderNoop,
		fleetv1alpha1.ProvisionProviderYaidap:
		return nil
	default:
		return fmt.Errorf("unknown provision provider %q", provider)
	}
}

// ValidateAction returns an error if action is unknown.
func ValidateAction(action string) error {
	switch strings.TrimSpace(action) {
	case fleetv1alpha1.ProvisionActionInstallBM,
		fleetv1alpha1.ProvisionActionCreateVM,
		fleetv1alpha1.ProvisionActionPlanVM,
		fleetv1alpha1.ProvisionActionDestroy,
		fleetv1alpha1.ProvisionActionDiscover,
		fleetv1alpha1.ProvisionActionDiscoverPXE,
		fleetv1alpha1.ProvisionActionStatus:
		return nil
	default:
		return fmt.Errorf("unknown provision action %q", action)
	}
}

// RequiresConfirm reports whether the action is destructive / PXE-arming class.
func RequiresConfirm(action string) bool {
	switch strings.TrimSpace(action) {
	case fleetv1alpha1.ProvisionActionInstallBM,
		fleetv1alpha1.ProvisionActionDiscoverPXE,
		fleetv1alpha1.ProvisionActionDestroy:
		return true
	default:
		return false
	}
}

// YaidapInvocation is the default remote ansible-playbook sketch for an action.
type YaidapInvocation struct {
	Playbook  string
	Tags      []string
	ExtraArgs []string
	ConfirmE  string // e.g. tempxe_confirm=true when confirm required
}

// DefaultYaidapInvocation returns illustrative YAIDAP argv pieces for an action.
func DefaultYaidapInvocation(action string, confirm bool) (YaidapInvocation, error) {
	if err := ValidateAction(action); err != nil {
		return YaidapInvocation{}, err
	}
	switch action {
	case fleetv1alpha1.ProvisionActionDiscover:
		return YaidapInvocation{
			Playbook: "playbooks/discover-hosts.yml",
			Tags:     []string{"check_host_details"},
		}, nil
	case fleetv1alpha1.ProvisionActionDiscoverPXE:
		inv := YaidapInvocation{
			Playbook: "playbooks/configure-tempxe.yml",
			Tags:     []string{"configure_tempxe", "tempxe_start"},
		}
		if confirm {
			inv.ConfirmE = "tempxe_confirm=true"
			inv.ExtraArgs = []string{"-e", "install_os=true"}
		}
		return inv, nil
	case fleetv1alpha1.ProvisionActionCreateVM:
		return YaidapInvocation{
			Playbook: "playbooks/provision-vms.yml",
			Tags:     []string{"create", "never"},
		}, nil
	case fleetv1alpha1.ProvisionActionPlanVM:
		return YaidapInvocation{
			Playbook: "playbooks/plan-vm-placement.yml",
			Tags:     []string{"plan"},
		}, nil
	case fleetv1alpha1.ProvisionActionDestroy:
		inv := YaidapInvocation{
			Playbook: "playbooks/provision-vms.yml",
			Tags:     []string{"destroy"},
		}
		if confirm {
			inv.ExtraArgs = append(inv.ExtraArgs, "-e", "confirm_destroy=true")
		}
		return inv, nil
	case fleetv1alpha1.ProvisionActionInstallBM:
		inv := YaidapInvocation{
			Playbook: "playbooks/configure-tempxe.yml",
			Tags:     []string{"configure_tempxe", "tempxe_start"},
		}
		if confirm {
			inv.ConfirmE = "tempxe_confirm=true"
			inv.ExtraArgs = []string{"-e", "install_os=true"}
		}
		return inv, nil
	case fleetv1alpha1.ProvisionActionStatus:
		return YaidapInvocation{
			Playbook: "playbooks/configure-tempxe.yml",
			Tags:     []string{"tempxe_status"},
		}, nil
	default:
		return YaidapInvocation{}, fmt.Errorf("unsupported action %q", action)
	}
}

// ValidateYaidapBinding checks jumphost + inventory for the yaidap provider.
func ValidateYaidapBinding(spec *fleetv1alpha1.SiteProvisionSpec) error {
	if spec == nil {
		return fmt.Errorf("yaidap requires spec.provision")
	}
	if strings.TrimSpace(spec.Jumphost) == "" {
		return fmt.Errorf("yaidap requires provision.jumphost")
	}
	if strings.TrimSpace(spec.InventoryPath) == "" {
		return fmt.Errorf("yaidap requires provision.inventoryPath")
	}
	if strings.TrimSpace(spec.SSHSecretRef) == "" {
		return fmt.Errorf("yaidap requires provision.sshSecretRef")
	}
	return nil
}
