package script

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderBundleModeSSHPeerSeed(t *testing.T) {
	out, err := Render(Params{
		SiteName:              "homelab",
		ClusterName:           "virt02",
		EnrollmentToken:       "token",
		HubWireguardIP:        "10.10.0.1",
		EnrollmentBaseURL:     "http://10.10.0.1:30912/fleet/api/v1alpha1",
		WGPublicKey:           "wgpub",
		RockyMajor:            "9",
		BundleMode:            true,
		WireguardEndpointHost: "wg.example.com",
		WireguardEndpointPort: "51820",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"is_local_mgmt_host()",
		"inventory_mgmt_ips()",
		"pip3 install --quiet netaddr",
		"curl -s -o /tmp/fleet-cluster-enroll.json",
		"Skipping SSH peer seed for local host",
		`ssh -i "${key_file}"`,
		"BatchMode=yes",
		"sudo -u linux ssh",
		`wireguard_endpoint_host=wg.example.com`,
		`wireguard_endpoint_port=51820`,
		// Fixed section parser: clear on any new [section], never leak into [hub].
		"/^\\[/ { sect=0; next }",
	} {
		if strings.Contains(want, "sudo -u linux ssh") {
			if strings.Contains(out, want) {
				t.Fatalf("bundle bootstrap still uses %q", want)
			}
			continue
		}
		if !strings.Contains(out, want) {
			t.Fatalf("bundle bootstrap missing %q", want)
		}
	}
	if strings.Contains(out, `if (sect !~ /fleet_cluster_(control|workers)/) sect=""`) {
		t.Fatal("bundle bootstrap still has buggy inventory section awk that leaks [hub]")
	}
}

// Sample inventory mirrors a single-node FCJ bundle: control node plus hub/site groups.
const sampleBundleInventory = `[fleet_cluster_control]
lab-int04 ansible_host=192.168.1.50 ansible_user=linux wireguard_address=10.10.0.12/32 fleet_node_mgmt_address=192.168.1.50 k3s_role=server

[fleet_cluster_nodes:children]
fleet_cluster_control

[fleet_cluster_nodes:vars]
fleet_hub_wireguard_ip=10.10.0.1

[site_bootstrap]
lab-int04 ansible_host=192.168.1.50 ansible_user=linux

[lab-int04]
lab-int04 ansible_host=192.168.1.50 ansible_user=linux

[hub]
umkim-hub ansible_host=10.10.0.1 ansible_user=linux
`

func TestInventoryMgmtIPsAwkIgnoresHubAndDuplicateGroups(t *testing.T) {
	out, err := Render(Params{
		SiteName:              "homelab",
		ClusterName:           "lab-int04",
		EnrollmentToken:       "token",
		HubWireguardIP:        "10.10.0.1",
		EnrollmentBaseURL:     "http://10.10.0.1:30912/fleet/api/v1alpha1",
		WGPublicKey:           "wgpub",
		RockyMajor:            "9",
		BundleMode:            true,
		WireguardEndpointHost: "wg.example.com",
		WireguardEndpointPort: "51820",
		KubevirtBridgeEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(out, "inventory_mgmt_ips() {")
	if start < 0 {
		t.Fatal("inventory_mgmt_ips missing")
	}
	awkStart := strings.Index(out[start:], "awk '")
	if awkStart < 0 {
		t.Fatal("awk missing in inventory_mgmt_ips")
	}
	awkStart += start + len("awk '")
	awkEnd := strings.Index(out[awkStart:], "' \"${BUNDLE_DIR}/inventory.ini\"")
	if awkEnd < 0 {
		t.Fatal("awk end marker missing")
	}
	awkProg := out[awkStart : awkStart+awkEnd]

	dir := t.TempDir()
	invPath := filepath.Join(dir, "inventory.ini")
	if err := os.WriteFile(invPath, []byte(sampleBundleInventory), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("awk", awkProg, invPath)
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("awk failed: %v\n%s\nprog:\n%s", err, got, awkProg)
	}
	lines := strings.Fields(strings.TrimSpace(string(got)))
	if len(lines) != 1 || lines[0] != "192.168.1.50" {
		t.Fatalf("expected only cluster mgmt IP 192.168.1.50, got %q", strings.TrimSpace(string(got)))
	}
}
