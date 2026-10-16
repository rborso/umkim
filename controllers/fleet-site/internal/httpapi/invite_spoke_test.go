package httpapi

import (
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func inviteTestCluster(directory string, enabled bool) *fleetv1alpha1.FleetClusterJoin {
	fcj := &fleetv1alpha1.FleetClusterJoin{}
	fcj.Spec.IngressAuth.Enabled = enabled
	fcj.Spec.IngressAuth.Directory = directory
	return fcj
}

// Only a cluster that runs its own LLDAP needs the spoke-side redeem path;
// clusters bound to the hub directory (and clusters without ingress auth) use
// the hub directory itself.
func TestClusterUsesOwnDirectory(t *testing.T) {
	cases := []struct {
		name    string
		fcj     *fleetv1alpha1.FleetClusterJoin
		wantOwn bool
	}{
		{"local", inviteTestCluster("", true), true},
		{"explicit local", inviteTestCluster("local", true), true},
		{"hub directory", inviteTestCluster("hub", true), false},
		{"ingress auth disabled", inviteTestCluster("local", false), false},
		{"nil cluster", nil, false},
	}
	for _, tc := range cases {
		if got := clusterUsesOwnDirectory(tc.fcj); got != tc.wantOwn {
			t.Errorf("%s: clusterUsesOwnDirectory = %v, want %v", tc.name, got, tc.wantOwn)
		}
	}
}

// The invitee's password and profile never appear in the Job spec: they travel
// in a Secret that the redeem path deletes afterwards.
func TestBuildInviteRedeemSecret(t *testing.T) {
	sec := buildInviteRedeemSecret("labuser", "s3cret-password", "Lab User", "labuser@example.com", []string{"movies", "tv"})
	if sec.Name != inviteRedeemSecretName || sec.Namespace != inviteRedeemNamespace {
		t.Fatalf("secret identity = %s/%s", sec.Namespace, sec.Name)
	}
	if sec.StringData["INVITE_USERNAME"] != "labuser" {
		t.Fatalf("username = %q", sec.StringData["INVITE_USERNAME"])
	}
	if sec.StringData["INVITE_PASSWORD"] != "s3cret-password" {
		t.Fatalf("password not carried")
	}
	if got := sec.StringData["INVITE_GROUPS"]; got != "movies tv" {
		t.Fatalf("groups = %q, want space separated", got)
	}
}

func TestBuildInviteRedeemJob(t *testing.T) {
	job := buildInviteRedeemJob("registry.example/lldap:stable")
	if job.Name != inviteRedeemJobName || job.Namespace != inviteRedeemNamespace {
		t.Fatalf("job identity = %s/%s", job.Namespace, job.Name)
	}
	if job.Spec.TTLSecondsAfterFinished == nil || *job.Spec.TTLSecondsAfterFinished <= 0 {
		t.Fatal("job must clean itself up")
	}
	if len(job.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("containers = %d", len(job.Spec.Template.Spec.Containers))
	}
	c := job.Spec.Template.Spec.Containers[0]
	if c.Image != "registry.example/lldap:stable" {
		t.Fatalf("image = %q", c.Image)
	}
	env := map[string]corev1.EnvVar{}
	for _, e := range c.Env {
		env[e.Name] = e
	}
	// The admin password comes from the chart's existing directory secret ...
	admin, ok := env["LLDAP_ADMIN_PASSWORD"]
	if !ok || admin.ValueFrom == nil || admin.ValueFrom.SecretKeyRef == nil {
		t.Fatal("LLDAP_ADMIN_PASSWORD must come from a Secret")
	}
	if admin.ValueFrom.SecretKeyRef.Name != inviteRedeemAdminSecret || admin.ValueFrom.SecretKeyRef.Key != inviteRedeemAdminKey {
		t.Fatalf("admin ref = %s/%s", admin.ValueFrom.SecretKeyRef.Name, admin.ValueFrom.SecretKeyRef.Key)
	}
	// ... and everything about the invitee comes from the redeem Secret.
	for _, key := range []string{"INVITE_USERNAME", "INVITE_PASSWORD", "INVITE_DISPLAY_NAME", "INVITE_EMAIL", "INVITE_GROUPS"} {
		e, ok := env[key]
		if !ok || e.ValueFrom == nil || e.ValueFrom.SecretKeyRef == nil || e.ValueFrom.SecretKeyRef.Name != inviteRedeemSecretName {
			t.Fatalf("%s must come from the redeem Secret", key)
		}
		if e.Value != "" {
			t.Fatalf("%s must not be inlined in the pod spec", key)
		}
	}
	// The script must not interpolate invitee values: they are read from the env.
	for _, leak := range []string{"${INVITE_USERNAME}\":\"", "INVITE_PASSWORD="} {
		if strings.Contains(c.Args[0], leak) {
			t.Fatalf("script interpolates invitee input: %s", leak)
		}
	}
	if !strings.Contains(c.Args[0], "lldap_set_password") {
		t.Fatal("script must set the password")
	}
	// An existing username must fail, never be adopted: continuing would reset
	// that account's password and add it to the invite's groups, which is account
	// takeover for anyone who can name an existing user.
	if !strings.Contains(c.Args[0], "already exists in this directory") {
		t.Fatal("script must refuse an existing user")
	}
	if strings.Contains(c.Args[0], "completing the remaining steps") {
		t.Fatal("script must not resume on an existing user")
	}
	// A transient directory error must not strand a freshly created user with no
	// group and no password.
	if !strings.Contains(c.Args[0], "gql_retry") {
		t.Fatal("script must retry transient directory errors")
	}
	if !strings.Contains(c.Args[0], "set -uo pipefail") {
		t.Fatal("script must not use set -e: retried calls report through their own handling")
	}
}
