package yaidap

import (
	"fmt"
	"regexp"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/provision"
)

var yaidapSyncRevRE = regexp.MustCompile(`FLEET_YAIDAP_SYNC_REV=([0-9a-f]{7,40})`)

// ValidateYaidapRepoSync checks binding fields required to sync YAIDAP on the jumphost.
func ValidateYaidapRepoSync(spec *fleetv1alpha1.SiteProvisionSpec) error {
	if spec == nil {
		return fmt.Errorf("yaidap repo sync requires spec.provision")
	}
	if err := provision.ValidateYaidapBinding(spec); err != nil {
		return err
	}
	if strings.TrimSpace(spec.YaidapRepo) == "" {
		return fmt.Errorf("yaidap repo sync requires provision.yaidapRepo")
	}
	if repoNeedsDeployKey(spec.YaidapRepo) && strings.TrimSpace(spec.YaidapGitSecretRef) == "" {
		return fmt.Errorf("yaidap repo sync requires provision.yaidapGitSecretRef for SSH git URLs")
	}
	return nil
}

func repoNeedsDeployKey(repo string) bool {
	r := strings.TrimSpace(repo)
	return strings.HasPrefix(r, "git@") || strings.HasPrefix(r, "ssh://")
}

// EffectiveYaidapRef returns the checkout ref, defaulting to main.
func EffectiveYaidapRef(spec *fleetv1alpha1.SiteProvisionSpec) string {
	if spec == nil {
		return "main"
	}
	ref := strings.TrimSpace(spec.YaidapRef)
	if ref == "" {
		return "main"
	}
	return ref
}

// YaidapRepoSyncExtraVars returns connector -e args for sync_yaidap.
func YaidapRepoSyncExtraVars(spec *fleetv1alpha1.SiteProvisionSpec) []string {
	root := strings.TrimSpace(spec.YaidaPRoot)
	if root == "" {
		root = "~/yaidap"
	}
	extra := []string{
		"-e", "fleet_provision_action=" + connectors.YaidapActionSync,
		"-e", "fleet_provision_inventory_path=" + strings.TrimSpace(spec.InventoryPath),
		"-e", "fleet_provision_yaidap_root=" + root,
		"-e", "fleet_provision_playbook=site-provision-connector.yml",
		"-e", "fleet_provision_yaidap_repo=" + strings.TrimSpace(spec.YaidapRepo),
		"-e", "fleet_provision_yaidap_ref=" + EffectiveYaidapRef(spec),
	}
	if strings.TrimSpace(spec.YaidapGitSecretRef) != "" {
		extra = append(extra, "-e", "fleet_yaidap_git_key_mount=true")
	}
	return extra
}

// ParseSyncedRevision extracts the checked-out git SHA from connector job logs.
func ParseSyncedRevision(logs string) string {
	match := yaidapSyncRevRE.FindStringSubmatch(logs)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

// GitKeyFileName picks the private key filename inside a mounted git secret.
func GitKeyFileName(keys map[string][]byte) string {
	for _, name := range []string{"id_ed25519", "deploy_key", "id_rsa"} {
		if v, ok := keys[name]; ok && len(v) > 0 {
			return name
		}
	}
	return ""
}
