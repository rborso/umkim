package provision

import (
	"regexp"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

const yaidapVmLocationMarker = "FLEET_YAIDAP_VM_LOCATION="

var yaidapResolvedVmLocationRE = regexp.MustCompile(`FLEET_YAIDAP_VM_LOCATION=([a-z0-9](?:[-a-z0-9]*[a-z0-9])?)`)

// ActionSyncsYaidapInventory reports actions that push connector inventory to the jumphost before YAIDAP runs.
func ActionSyncsYaidapInventory(action string) bool {
	switch strings.TrimSpace(action) {
	case fleetv1alpha1.ProvisionActionCreateVM,
		fleetv1alpha1.ProvisionActionPlanVM,
		fleetv1alpha1.ProvisionActionDestroy,
		fleetv1alpha1.ProvisionActionInstallBM:
		return true
	default:
		return false
	}
}

// YaidapSyncExtraVars returns connector extra vars that sync hub host fields to YAIDAP inventory.
func YaidapSyncExtraVars(rec fleetv1alpha1.SiteHostRecord) []string {
	if rec.Kind != fleetv1alpha1.SiteHostKindVM {
		return nil
	}
	extra := []string{
		"-e", "fleet_provision_sync_host=true",
		"-e", "fleet_provision_disk_gi=40",
	}
	if loc := strings.TrimSpace(rec.VmLocation); loc != "" {
		extra = append(extra, "-e", "fleet_provision_vm_location="+loc)
	}
	if addr := strings.TrimSpace(rec.AnsibleHost); addr != "" {
		extra = append(extra, "-e", "fleet_provision_ansible_host="+addr)
	}
	if rec.Flavor != nil {
		if memory := strings.TrimSpace(rec.Flavor["memory"]); memory != "" {
			extra = append(extra, "-e", "fleet_provision_memory="+memory)
		}
		if cores := strings.TrimSpace(rec.Flavor["cores"]); cores != "" {
			extra = append(extra, "-e", "fleet_provision_cores="+cores)
		}
	}
	if osFamily := strings.TrimSpace(rec.OSFamily); osFamily != "" {
		extra = append(extra, "-e", "fleet_provision_os="+osFamily)
	}
	return extra
}

// ParseResolvedVmLocation extracts a concrete hypervisor from connector job logs.
func ParseResolvedVmLocation(logs string) string {
	match := yaidapResolvedVmLocationRE.FindStringSubmatch(logs)
	if len(match) < 2 {
		return ""
	}
	loc := strings.TrimSpace(match[1])
	if loc == "" {
		return ""
	}
	lower := strings.ToLower(loc)
	if lower == "least-utilized" || lower == "least_utilized" || lower == "auto" {
		return ""
	}
	return loc
}
