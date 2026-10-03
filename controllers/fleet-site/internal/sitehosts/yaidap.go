package sitehosts

import (
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

var vmLocationScheduleTokens = map[string]struct{}{
	"":                 {},
	"least-utilized":   {},
	"least_utilized":   {},
	"auto":             {},
}

// VmLocationIsScheduleToken reports whether loc defers placement to YAIDAP scheduling.
func VmLocationIsScheduleToken(loc string) bool {
	_, ok := vmLocationScheduleTokens[strings.ToLower(strings.TrimSpace(loc))]
	return ok
}

// ValidateVmLocation checks vm_location for VM hosts.
func ValidateVmLocation(kind, loc string) error {
	if strings.TrimSpace(kind) != fleetv1alpha1.SiteHostKindVM {
		return nil
	}
	loc = strings.TrimSpace(loc)
	if loc == "" || VmLocationIsScheduleToken(loc) {
		return nil
	}
	if len(loc) > 63 || !hostNamePattern.MatchString(loc) {
		return fmt.Errorf("invalid vm_location %q (hypervisor hostname or least-utilized)", loc)
	}
	return nil
}
