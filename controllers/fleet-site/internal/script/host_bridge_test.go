package script

import (
	"strings"
	"testing"
)

func TestRenderHostBridgeDisabled(t *testing.T) {
	out, err := RenderHostBridge(HostBridgeParams{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("expected empty script when disabled, got %d bytes", len(out))
	}
}

func TestRenderHostBridgeEnabled(t *testing.T) {
	out, err := RenderHostBridge(HostBridgeParams{
		Enabled:    true,
		BridgeName: "n0p0_mgmt_nw",
		SlaveNic:   "enp0s31f6",
		KeepIP:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"#!/usr/bin/env bash",
		`BRIDGE_NAME="n0p0_mgmt_nw"`,
		`SLAVE_NIC_OVERRIDE="enp0s31f6"`,
		`KEEP_IP="true"`,
		"MGMT_IP=",
		"iface_for_ip",
		"assert_not_already_bridged",
		"nmcli connection add type bridge",
		"fleet-kubevirt-bridge",
		"usage: $0 <mgmt_ipv4>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("host bridge script missing %q", want)
		}
	}
}

func TestRenderBundleModeKubevirtBridgeHooks(t *testing.T) {
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
		KubevirtBridgeEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"run_kubevirt_host_bridge_local",
		"run_kubevirt_host_bridge_peers",
		"configure-kubevirt-bridge.sh",
		"local_mgmt_ip_from_inventory",
		"bash \"${script}\" \"${mgmt_ip}\"",
		"bash ${remote} ${host}",
		"systemd-run",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("bundle bootstrap missing bridge hook %q", want)
		}
	}
	// Bridge runs before wireguard so mgmt IP is stable for peer SSH.
	localIdx := strings.Index(out, "run_kubevirt_host_bridge_local")
	wgIdx := strings.Index(out, "run_ansible wireguard-install.yml")
	if localIdx < 0 || wgIdx < 0 || localIdx > wgIdx {
		t.Fatalf("expected host bridge before wireguard-install (local=%d wg=%d)", localIdx, wgIdx)
	}
}
