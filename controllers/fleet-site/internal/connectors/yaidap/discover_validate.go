package yaidap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/nodeops"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ErrDestroyClusterConflict is returned when destroy is blocked by an active FleetClusterJoin.
var ErrDestroyClusterConflict = errors.New("destroy blocked by active cluster membership")

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
	listed, err := pxeDiscoverySubjectListed(data, subject)
	if err != nil {
		return err
	}
	if !listed {
		return fmt.Errorf("discover_pxe requires %q in hypervisors.yml or bare_metals.yml", subject)
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

func pxeDiscoverySubjectListed(data map[string]string, subject string) (bool, error) {
	if hvRaw, ok := connectors.GetInventoryFile(data, "hypervisors.yml"); ok && strings.TrimSpace(hvRaw) != "" {
		hvs, err := ParseHypervisorsYAML(hvRaw)
		if err != nil {
			return false, err
		}
		if hypervisorInIndex(subject, hvs) {
			return true, nil
		}
	}
	if bmRaw, ok := connectors.GetInventoryFile(data, "bare_metals.yml"); ok && strings.TrimSpace(bmRaw) != "" {
		bm, err := ParseBareMetalsYAML(bmRaw)
		if err != nil {
			return false, err
		}
		if bareMetalInIndex(subject, bm) {
			return true, nil
		}
	}
	return false, nil
}

// DiscoverySuccessMessage returns the post-job hint for discovery actions.
func DiscoverySuccessMessage(action string) string {
	switch strings.TrimSpace(action) {
	case fleetv1alpha1.ProvisionActionDiscover, fleetv1alpha1.ProvisionActionDiscoverPXE:
		return "YAIDAP discovery succeeded; run import preview to promote generated drafts (resolve CHANGE_ME placeholders before sync/provision)"
	case fleetv1alpha1.ProvisionActionPrepareHypervisor:
		return "Hypervisor prepare succeeded (libvirt bridges and packages)"
	case fleetv1alpha1.ProvisionActionPrepareGPUPassthrough:
		return "GPU passthrough host prepare succeeded (VFIO bind; host NVIDIA driver removed when configured)"
	case fleetv1alpha1.ProvisionActionPrepareHypervisorGPU:
		return "Hypervisor and GPU passthrough host prepare succeeded"
	default:
		return ""
	}
}

const destroySuccessMessage = "VM destroyed; guest remains in inventory"

// DestroySuccessMessage returns the post-job message for destroy actions.
func DestroySuccessMessage() string {
	return destroySuccessMessage
}

// ValidateDestroyAction checks preconditions for tearing down a libvirt VM while keeping inventory.
func ValidateDestroyAction(ctx context.Context, c client.Client, ns, siteName, subject string, invHost InventoryHostView, guestFound bool) error {
	if !guestFound || invHost.External {
		return fmt.Errorf("destroy requires an existing VM guest in guests.yml")
	}
	kind := strings.TrimSpace(invHost.Kind)
	if kind == "" {
		kind = fleetv1alpha1.SiteHostKindVM
	}
	if kind != fleetv1alpha1.SiteHostKindVM {
		return fmt.Errorf("destroy applies to VM guests only")
	}
	phase := strings.TrimSpace(invHost.Status.Phase)
	if phase != fleetv1alpha1.SiteHostPhaseReady && phase != fleetv1alpha1.SiteHostPhaseFailed {
		if phase == "" {
			phase = fleetv1alpha1.SiteHostPhaseDeclared
		}
		return fmt.Errorf("destroy requires guest phase Ready or Failed (current %q)", phase)
	}
	clustersDoc, err := LoadClustersFile(ctx, c, ns, siteName)
	if err != nil {
		return err
	}
	for clusterName, spec := range clustersDoc.Clusters {
		for _, member := range spec.Members {
			if member != subject {
				continue
			}
			fcj, err := nodeops.FindFleetClusterJoin(ctx, c, ns, clusterName)
			if err != nil {
				continue
			}
			if fcj.Status.Phase != fleetv1alpha1.JoinPhaseDeleting {
				return fmt.Errorf("%w: guest is a member of cluster %q; delete or unjoin the fleet cluster first", ErrDestroyClusterConflict, clusterName)
			}
		}
	}
	return nil
}
