package yaidap

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidateDiscoverAction checks preconditions for SSH hardware discovery.
func ValidateDiscoverAction(ctx context.Context, c client.Client, ns, siteName, subject string, invHost InventoryHostView, guestFound bool) error {
	if !guestFound {
		return fmt.Errorf("host %q not found in inventory", subject)
	}
	addr := strings.TrimSpace(invHost.AnsibleHost)
	if addr == "" {
		addr, _ = GuestAddress(ctx, c, ns, siteName, subject)
	}
	if strings.TrimSpace(addr) == "" {
		return fmt.Errorf("discover requires ansible_host for %q", subject)
	}
	return nil
}

// ValidateDiscoverPXEAction checks preconditions for TempXE PXE discovery.
func ValidateDiscoverPXEAction(ctx context.Context, c client.Client, ns, siteName, subject string) error {
	data, err := connectors.Load(ctx, c, ns, siteName, provider)
	if err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("inventory store is empty")
	}
	data = connectors.NormalizeMetaKeys(data)
	hvRaw, ok := connectors.GetInventoryFile(data, "hypervisors.yml")
	if !ok {
		return fmt.Errorf("discover_pxe requires hypervisors.yml")
	}
	hvs, err := ParseHypervisorsYAML(hvRaw)
	if err != nil {
		return err
	}
	if !hypervisorInIndex(subject, hvs) {
		return fmt.Errorf("discover_pxe requires %q in hypervisors.yml", subject)
	}
	hostVars, ok := connectors.GetInventoryFile(data, "host_vars/"+subject+".yml")
	if !ok || strings.TrimSpace(hostVars) == "" {
		return fmt.Errorf("discover_pxe requires host_vars/%s.yml", subject)
	}
	if !hostVarsPXEDiscoveryEnabled(hostVars) {
		return fmt.Errorf("discover_pxe requires tempxe_discovery.enabled: true in host_vars/%s.yml", subject)
	}
	if !hostVarsHasMAC(hostVars) {
		return fmt.Errorf("discover_pxe requires at least one mac_address in host_vars/%s.yml uplinks or interfaces", subject)
	}
	return nil
}

func hostVarsPXEDiscoveryEnabled(content string) bool {
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return false
	}
	discovery, ok := doc["tempxe_discovery"].(map[string]interface{})
	if !ok {
		return false
	}
	enabled, ok := discovery["enabled"].(bool)
	return ok && enabled
}

func hostVarsHasMAC(content string) bool {
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return false
	}
	for _, key := range []string{"uplinks", "interfaces"} {
		items, ok := doc[key].([]interface{})
		if !ok {
			continue
		}
		for _, item := range items {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if mac, ok := m["mac_address"].(string); ok && strings.TrimSpace(mac) != "" {
				return true
			}
		}
	}
	return false
}

// DiscoverySuccessMessage returns the post-job hint for discovery actions.
func DiscoverySuccessMessage(action string) string {
	switch strings.TrimSpace(action) {
	case fleetv1alpha1.ProvisionActionDiscover, fleetv1alpha1.ProvisionActionDiscoverPXE:
		return "YAIDAP discovery succeeded; run import preview to promote generated drafts (resolve CHANGE_ME placeholders before sync/provision)"
	default:
		return ""
	}
}
