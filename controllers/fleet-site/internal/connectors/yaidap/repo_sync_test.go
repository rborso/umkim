package yaidap

import (
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestValidateYaidapRepoSync(t *testing.T) {
	spec := &fleetv1alpha1.SiteProvisionSpec{
		Provider:      fleetv1alpha1.ProvisionProviderYaidap,
		Jumphost:      "linux@jumphost",
		SSHSecretRef:  "fleet-site-homelab-provision-ssh",
		InventoryPath: "/inventory/homelab",
		YaidapRepo:    "https://github.com/rborso/yaidap.git",
	}
	if err := ValidateYaidapRepoSync(spec); err != nil {
		t.Fatal(err)
	}
	spec.YaidapRepo = "git@github.com:rborso/yaidap.git"
	if err := ValidateYaidapRepoSync(spec); err == nil {
		t.Fatal("expected error without git secret")
	}
	spec.YaidapGitSecretRef = "fleet-site-homelab-yaidap-git"
	if err := ValidateYaidapRepoSync(spec); err != nil {
		t.Fatal(err)
	}
}

func TestParseSyncedRevision(t *testing.T) {
	const sha = "0429c71abcd1234ef5678901234abcd1234abcd"
	rev := ParseSyncedRevision("TASK ok\nFLEET_YAIDAP_SYNC_REV=" + sha + "\n")
	if rev != sha {
		t.Fatalf("rev=%q", rev)
	}
}

func TestYaidapRepoSyncExtraVars(t *testing.T) {
	spec := &fleetv1alpha1.SiteProvisionSpec{
		InventoryPath:      "/inv",
		YaidaPRoot:         "/opt/yaidap",
		YaidapRepo:         "git@github.com:org/yaidap.git",
		YaidapGitSecretRef: "git-secret",
	}
	extra := YaidapRepoSyncExtraVars(spec)
	joined := strings.Join(extra, " ")
	for _, want := range []string{"sync_yaidap", "/opt/yaidap", "git@github.com:org/yaidap.git", "fleet_yaidap_git_key_mount=true"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
}
