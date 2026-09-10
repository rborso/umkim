package yaidap

import (
	"testing"
)

func TestHostVarsPXEDiscoveryEnabled(t *testing.T) {
	content := `tempxe_discovery:
  enabled: true
uplinks:
  - mac_address: "aa:bb:cc:dd:ee:ff"
`
	if !hostVarsPXEDiscoveryEnabled(content) {
		t.Fatal("expected enabled")
	}
	if !hostVarsHasMAC(content) {
		t.Fatal("expected mac")
	}
}

func TestHostVarsPXEDiscoveryDisabled(t *testing.T) {
	content := `tempxe_discovery:
  enabled: false
uplinks:
  - mac_address: "aa:bb:cc:dd:ee:ff"
`
	if hostVarsPXEDiscoveryEnabled(content) {
		t.Fatal("expected disabled")
	}
}

func TestDiscoverySuccessMessage(t *testing.T) {
	if DiscoverySuccessMessage("discover") == "" {
		t.Fatal("expected discover message")
	}
	if DiscoverySuccessMessage("create_vm") != "" {
		t.Fatal("expected empty for create_vm")
	}
}
