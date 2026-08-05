package jobs

import (
	"strings"
	"testing"
)

func TestProviderJobMountsHubSecrets(t *testing.T) {
	job := ProviderJob("fleet-console", "fleet-cluster-provider-helm-install-x", "x", "site1", "helm", "install", "img:tag", "ssh", "inv", nil)
	pod := job.Spec.Template.Spec
	foundVol := false
	for _, v := range pod.Volumes {
		if v.Name == "hub-secrets" && v.Secret != nil && v.Secret.SecretName == HubAnsibleSecretsName {
			foundVol = true
			break
		}
	}
	if !foundVol {
		t.Fatal("expected hub-secrets volume")
	}
	foundMount := false
	for _, m := range pod.Containers[0].VolumeMounts {
		if m.Name == "hub-secrets" && m.MountPath == "/secrets/hub" {
			foundMount = true
			break
		}
	}
	if !foundMount {
		t.Fatal("expected hub-secrets mount at /secrets/hub")
	}
	cmd := job.Spec.Template.Spec.Containers[0].Command
	want := "-e"
	wantFile := "@/secrets/hub/ansible_vars.yml"
	for i := 0; i < len(cmd)-1; i++ {
		if cmd[i] == want && cmd[i+1] == wantFile {
			return
		}
	}
	t.Fatalf("expected -e %s in command, got %v", wantFile, cmd)
}

func TestWgJobShellAddsOverlayRoute(t *testing.T) {
	cmd := wgJobShell([]WGPeer{{PublicKey: "abc", AllowedIP: "10.10.0.12"}}, nil)
	if !strings.Contains(cmd, `ip route replace 10.10.0.12/32 dev "$IFACE"`) {
		t.Fatalf("expected overlay route in wg sync shell, got: %s", cmd)
	}
}

func TestOverlayConnectivityJobContainsProbe(t *testing.T) {
	job := OverlayConnectivityJob("ns", "j", "pubkey", "10.10.0.12", 30191)
	cmd := job.Spec.Template.Spec.Containers[0].Args[0]
	if !strings.Contains(cmd, "overlay_ok") || !strings.Contains(cmd, "30191") {
		t.Fatalf("unexpected overlay job script: %s", cmd)
	}
	if strings.Contains(cmd, "%!") || !strings.Contains(cmd, "date +%s") {
		t.Fatalf("fmt.Sprintf must not consume date +%%s verb: %s", cmd)
	}
	if !strings.Contains(cmd, `[ "$AGE" -gt 90 ]`) {
		t.Fatalf("expected stale handshake threshold in script: %s", cmd)
	}
	if job.Spec.TTLSecondsAfterFinished == nil || *job.Spec.TTLSecondsAfterFinished != 300 {
		t.Fatalf("expected overlay job TTL 300s, got %v", job.Spec.TTLSecondsAfterFinished)
	}
}

func TestValidateJobNoHubSecrets(t *testing.T) {
	job := ValidateJob("fleet-console", "c", "c", "site1", "single", "img:tag", "ssh", "inv")
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.Name == "hub-secrets" {
			t.Fatal("validate job should not mount hub secrets")
		}
	}
}
