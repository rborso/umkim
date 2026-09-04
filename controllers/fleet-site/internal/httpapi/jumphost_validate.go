package httpapi

import (
	"fmt"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/ssh"
)

func validateJumphostValue(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if !strings.Contains(raw, "@") {
		return fmt.Errorf("invalid jumphost %q: use user@host:port", raw)
	}
	if _, ok := ssh.ParseJumphost(raw, "linux"); !ok {
		return fmt.Errorf("invalid jumphost %q: use user@host:port", raw)
	}
	return nil
}

func patchJumphostOnly(p clusterPatchBody) bool {
	if p.SiteID != "" || len(p.Capabilities) > 0 || len(p.Topology) > 0 ||
		len(p.CustomLabels) > 0 || p.IngressAuth != nil || p.Edge != nil {
		return false
	}
	return p.Jumphost != nil
}
