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

func TestChartPublishJob(t *testing.T) {
	job := ChartPublishJob(ChartPublishJobOpts{
		Namespace:      "fleet-console",
		JobName:        "fc-p-whoami",
		ChartName:      "whoami",
		Version:        "0.1.0",
		Source:         "upload",
		InboxFile:      "whoami.tgz",
		InboxHostPath:  "/var/lib/umkim/chart-inbox",
		ImagePolicy:    "external",
		DestOCI:        "oci://10.10.0.1:30443/charts",
		MirrorHost:     "10.10.0.1:30443/mirror",
		RunnerImage:    "img:tag",
		HubSecretsName: HubAnsibleSecretsName,
		ResultCM:       "fc-result-whoami",
	})
	pod := job.Spec.Template.Spec
	if pod.HostNetwork != true {
		t.Fatal("expected hostNetwork")
	}
	foundInbox, foundSecrets := false, false
	for _, v := range pod.Volumes {
		if v.Name == "inbox" && v.HostPath != nil {
			foundInbox = true
		}
		if v.Name == "hub-secrets" {
			foundSecrets = true
		}
	}
	if !foundInbox || !foundSecrets {
		t.Fatalf("inbox=%v secrets=%v", foundInbox, foundSecrets)
	}
	script := strings.Join(pod.Containers[0].Command, " ")
	if !strings.Contains(script, "helm push") {
		t.Fatal("expected helm push")
	}
	if !strings.Contains(script, "skopeo") {
		t.Fatal("expected skopeo copy path")
	}
}

func TestHelmAppJobInstallMounts(t *testing.T) {
	job := HelmAppJob(HelmAppJobOpts{
		Namespace:         "fleet-console",
		JobName:           "fa-i-whoami",
		Cluster:           "lab-single-0",
		Chart:             "oci://10.10.0.1:30443/charts/whoami",
		Version:           "0.1.0",
		Release:           "whoami",
		TargetNamespace:   "default",
		Action:            "install",
		RunnerImage:       "img:tag",
		ValuesConfigMap:   "fa-vals-whoami",
		ImageMapConfigMap: "fa-imap-whoami",
		RegistryHost:      "10.10.0.1:30443",
		HubSecretsName:    HubAnsibleSecretsName,
	})
	pod := job.Spec.Template.Spec
	if pod.HostNetwork != true {
		t.Fatal("expected hostNetwork")
	}
	if pod.Containers[0].Command[0] != "/bin/bash" {
		t.Fatalf("expected bash override of ansible-playbook entrypoint, got %v", pod.Containers[0].Command)
	}
	foundValues, foundSecrets, foundMap := false, false, false
	for _, v := range pod.Volumes {
		if v.Name == "values" && v.ConfigMap != nil && v.ConfigMap.Name == "fa-vals-whoami" {
			foundValues = true
		}
		if v.Name == "hub-secrets" && v.Secret != nil && v.Secret.SecretName == HubAnsibleSecretsName {
			foundSecrets = true
		}
		if v.Name == "imagemap" && v.ConfigMap != nil && v.ConfigMap.Name == "fa-imap-whoami" {
			foundMap = true
		}
	}
	if !foundValues || !foundSecrets || !foundMap {
		t.Fatalf("values=%v secrets=%v imagemap=%v", foundValues, foundSecrets, foundMap)
	}
	script := strings.Join(pod.Containers[0].Command, " ")
	if !strings.Contains(script, "helm upgrade --install") {
		t.Fatal("expected helm upgrade in script")
	}
	if !strings.Contains(script, "helm registry login") {
		t.Fatal("expected registry login in script")
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
