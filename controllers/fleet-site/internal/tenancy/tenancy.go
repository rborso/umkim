package tenancy

import (
	"fmt"
	"regexp"
	"strings"
)

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`)

// ValidateOrgPrefixedName ensures name starts with orgSlug-.
func ValidateOrgPrefixedName(orgSlug, name string) error {
	orgSlug = strings.TrimSpace(orgSlug)
	name = strings.TrimSpace(name)
	if orgSlug == "" {
		return fmt.Errorf("orgId is required")
	}
	if !slugPattern.MatchString(orgSlug) {
		return fmt.Errorf("invalid org slug %q", orgSlug)
	}
	prefix := orgSlug + "-"
	if !strings.HasPrefix(name, prefix) || len(name) <= len(prefix) {
		return fmt.Errorf("name %q must start with %q", name, prefix)
	}
	suffix := strings.TrimPrefix(name, prefix)
	suffixPattern := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	if !suffixPattern.MatchString(suffix) {
		return fmt.Errorf("name suffix %q must be DNS-1123 compliant", suffix)
	}
	return nil
}

const (
	LabelOrg  = "fleet.org"
	LabelPool = "fleet.pool"
	PoolShared    = "shared"
	PoolDedicated = "dedicated"
)

// EnsureDedicatedPool sets fleet.pool=dedicated when absent.
func EnsureDedicatedPool(labels map[string]string) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}
	if labels[LabelPool] == "" {
		labels[LabelPool] = PoolDedicated
	}
	return labels
}

// SetOrgLabel sets fleet.org on labels.
func SetOrgLabel(labels map[string]string, orgID string) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}
	if orgID != "" {
		labels[LabelOrg] = orgID
	}
	return labels
}
