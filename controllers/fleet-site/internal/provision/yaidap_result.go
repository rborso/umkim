package provision

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitehosts"
)

const IntegrationSchemaV1 = "yaidap.integration.v1"

var yaidapResultRE = regexp.MustCompile(`YAIDAP_RESULT=(\{.*\})`)

// IntegrationHandoff mirrors YAIDAP handoff fields in yaidap.integration.v1.
type IntegrationHandoff struct {
	Hostname    string `json:"hostname"`
	AnsibleHost string `json:"ansible_host"`
	AnsibleUser string `json:"ansible_user"`
	OSFamily    string `json:"os_family"`
}

// PlacementAssignment is one scheduled guest placement row.
type PlacementAssignment struct {
	Guest      string `json:"guest"`
	Hypervisor string `json:"hypervisor"`
	MemoryMib  int    `json:"memory_mib"`
	Source     string `json:"source"`
}

// IntegrationPlacement holds plan/create placement output.
type IntegrationPlacement struct {
	Assignments []PlacementAssignment `json:"assignments"`
	Unplaced    []string              `json:"unplaced"`
}

// IntegrationResult is the parsed YAIDAP_RESULT payload.
type IntegrationResult struct {
	Schema         string                `json:"schema"`
	Action         string                `json:"action"`
	Subject        string                `json:"subject"`
	Phase          string                `json:"phase"`
	OK             bool                  `json:"ok"`
	Message        string                `json:"message"`
	VmLocation     string                `json:"vm_location"`
	Handoff        *IntegrationHandoff   `json:"handoff"`
	Placement      *IntegrationPlacement `json:"placement"`
	GeneratedPaths []string              `json:"generated_paths"`
	Warnings       []string              `json:"warnings"`
	Errors         []string              `json:"errors"`
}

// ParseYaidapResult extracts the last yaidap.integration.v1 result from connector logs.
func ParseYaidapResult(logs string) (*IntegrationResult, bool) {
	matches := yaidapResultRE.FindAllStringSubmatch(logs, -1)
	if len(matches) == 0 {
		return nil, false
	}
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return nil, false
	}
	payload := strings.TrimSpace(last[1])
	var result IntegrationResult
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		payload = strings.ReplaceAll(payload, `\"`, `"`)
		if err := json.Unmarshal([]byte(payload), &result); err != nil {
			return nil, false
		}
	}
	if strings.TrimSpace(result.Schema) != IntegrationSchemaV1 {
		return nil, false
	}
	return &result, true
}

// ToHandoffHost maps integration handoff to ProvisionHandoffHost.
func (r *IntegrationResult) ToHandoffHost(siteID string) *fleetv1alpha1.ProvisionHandoffHost {
	if r == nil || r.Handoff == nil {
		return nil
	}
	h := r.Handoff
	if strings.TrimSpace(h.Hostname) == "" && strings.TrimSpace(h.AnsibleHost) == "" {
		return nil
	}
	return &fleetv1alpha1.ProvisionHandoffHost{
		Hostname:    strings.TrimSpace(h.Hostname),
		AnsibleHost: strings.TrimSpace(h.AnsibleHost),
		AnsibleUser: strings.TrimSpace(h.AnsibleUser),
		OSFamily:    strings.TrimSpace(h.OSFamily),
		SiteID:      siteID,
	}
}

// ConcreteVmLocation returns a persistable hypervisor name or empty when absent/schedule token.
func (r *IntegrationResult) ConcreteVmLocation() string {
	if r == nil {
		return ""
	}
	loc := strings.TrimSpace(r.VmLocation)
	if loc == "" || sitehosts.VmLocationIsScheduleToken(loc) {
		return ""
	}
	return loc
}

// FormatPlacementSummary renders plan_vm assignments for action history messages.
func FormatPlacementSummary(p *IntegrationPlacement) string {
	if p == nil || len(p.Assignments) == 0 {
		return ""
	}
	parts := make([]string, 0, len(p.Assignments))
	for _, a := range p.Assignments {
		guest := strings.TrimSpace(a.Guest)
		hv := strings.TrimSpace(a.Hypervisor)
		if guest == "" || hv == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s→%s", guest, hv))
	}
	if len(parts) == 0 {
		return ""
	}
	return "placement: " + strings.Join(parts, ", ")
}

// YaidapIntegrationExtraVars returns YAIDAP integration extra -e pairs for connector jobs.
func YaidapIntegrationExtraVars(action string, concreteVmLocation bool) []string {
	switch strings.TrimSpace(action) {
	case fleetv1alpha1.ProvisionActionInstallBM,
		fleetv1alpha1.ProvisionActionDiscover,
		fleetv1alpha1.ProvisionActionDiscoverPXE,
		fleetv1alpha1.ProvisionActionCreateVM,
		fleetv1alpha1.ProvisionActionPlanVM,
		fleetv1alpha1.ProvisionActionDestroy:
		out := []string{"-e", "yaidap_emit_result=true"}
		if action == fleetv1alpha1.ProvisionActionCreateVM && concreteVmLocation {
			out = append(out, "-e", "yaidap_recreate=true")
		}
		return out
	default:
		return nil
	}
}

// ActionUsesConnectionLocal reports actions whose YAIDAP playbook should run with
// --connection local on the jumphost for every play. Do not include install_bm:
// that playbook SSHs to the bare-metal target for reboot/wait while only TempXE
// arm plays set connection: local per play.
func ActionUsesConnectionLocal(action string) bool {
	switch strings.TrimSpace(action) {
	case fleetv1alpha1.ProvisionActionDiscoverPXE:
		return true
	default:
		return false
	}
}
