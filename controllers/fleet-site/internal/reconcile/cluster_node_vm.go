package reconcile

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
)

// nodeVMActorName is the inventory actor recorded for pipeline-generated guests.
const nodeVMActorName = "cluster-pipeline"

// defaultNodeVMDataDiskGi is the data disk given to a pipeline-created guest when
// Longhorn is enabled but the cluster does not state a minimum.
const defaultNodeVMDataDiskGi = 5

// nodeVMBootDiskOverheadGi is the space a guest disk loses to EFI/boot before the
// root LV. The bootstrap asserts the root filesystem is at least minRootDiskGi, so
// a disk sized exactly at minRootDiskGi fails; a 40 GiB vdisk reports a ~38 GiB
// root (sitecluster.ProvisionedVMMinRootDiskGi).
const nodeVMBootDiskOverheadGi = 2

// ensureNodeGuest declares the YAIDAP VM guest backing a cluster node that carries
// a vm spec, so the pipeline can then run create_vm for it. Declaring here (rather
// than requiring a hand-written entry first) is what removes the manual "add host"
// step: the guest name is the node hostname and the guest address is the node
// address.
//
// It is idempotent: an existing declaration with the same address is left
// untouched, and one with a different address is an error rather than a silent
// rewrite of a host an operator may have customised. Returns true when the
// inventory was changed.
func (r *ClusterJoinReconciler) ensureNodeGuest(
	ctx context.Context,
	site *fleetv1alpha1.FleetSite,
	fcj *fleetv1alpha1.FleetClusterJoin,
	node fleetv1alpha1.FleetClusterJoinNodeSpec,
) (bool, error) {
	if node.VM == nil {
		return false, nil
	}
	host := strings.TrimSpace(node.Hostname)
	if host == "" {
		return false, fmt.Errorf("node has no hostname; cannot declare a VM guest")
	}
	address := strings.TrimSpace(node.Address)
	clusterName := fcj.Spec.EffectiveClusterName(fcj.Name)

	paths := []string{
		"guests.yml",
		fmt.Sprintf("host_vars/%s.yml", host),
		fmt.Sprintf("%s.yml", clusterName),
	}
	current := map[string]string{}
	for _, p := range paths {
		content, found, err := yaidap.GetInventoryFileContent(ctx, r.Client, r.Cfg.Namespace, site.Name, p)
		if err != nil {
			return false, err
		}
		if found {
			current[p] = content
		}
	}
	existing, declared, err := declaredGuestAddress(current["guests.yml"], host)
	if err != nil {
		return false, err
	}
	if declared {
		if existing != "" && existing != address {
			return false, fmt.Errorf("guest %q is already declared with address %s, but the cluster node wants %s", host, existing, address)
		}
		return false, nil
	}

	catalog, _, _, err := yaidap.LoadBundle(ctx, r.Client, r.Cfg.Namespace, site.Name)
	if err != nil {
		return false, err
	}
	intent := yaidap.HostIntent{
		Kind:        "vm",
		Name:        host,
		AnsibleHost: address,
		Cluster:     clusterName,
		Hypervisor:  strings.TrimSpace(node.VM.Hypervisor),
		Cores:       node.VM.Cores,
		Memory:      node.VM.Memory,
		DiskSizeGi:  node.VM.DiskGi,
	}
	// The bootstrap validates the root filesystem against minRootDiskGi, so the
	// boot disk must exceed it by the EFI/boot overhead.
	if min := fcj.Spec.MinRootDiskGi; min > 0 && intent.DiskSizeGi < min+nodeVMBootDiskOverheadGi {
		intent.DiskSizeGi = min + nodeVMBootDiskOverheadGi
	}
	if data := nodeVMDataDiskGi(fcj, node); data > 0 {
		intent.Disks = []yaidap.HostDiskIntent{{SizeGi: data}}
	}
	files, err := yaidap.BuildHostFiles(current, intent, catalog)
	if err != nil {
		return false, err
	}
	if err := connectors.PutInventoryFiles(ctx, r.Client, r.Cfg.Namespace, site.Name,
		fleetv1alpha1.ProvisionProviderYaidap,
		connectors.FormatActor("Controller", nodeVMActorName),
		files, yaidap.ValidateInventoryFile); err != nil {
		return false, err
	}
	return true, nil
}

// nodeVMDataDiskGi returns the data disk for a pipeline-created guest. Longhorn
// needs a non-boot device, so derive it from the cluster storage spec when the
// node does not set one explicitly; forgetting it fails the bootstrap.
func nodeVMDataDiskGi(fcj *fleetv1alpha1.FleetClusterJoin, node fleetv1alpha1.FleetClusterJoinNodeSpec) int {
	if node.VM != nil && node.VM.DataDiskGi > 0 {
		return node.VM.DataDiskGi
	}
	storage := fcj.Spec.Storage
	if storage == nil || !storage.LonghornEnabled() {
		return 0
	}
	if storage.MinDataDiskGi > 0 {
		return storage.MinDataDiskGi
	}
	return defaultNodeVMDataDiskGi
}

// declaredGuestAddress reports the ansible_host recorded for a guest, when the
// guests.yml content declares it.
func declaredGuestAddress(guestsYAML, host string) (string, bool, error) {
	if strings.TrimSpace(guestsYAML) == "" {
		return "", false, nil
	}
	guests, err := yaidap.ParseGuestsYAML(guestsYAML)
	if err != nil {
		return "", false, err
	}
	entry, ok := guests.Guests.Hosts[host]
	if !ok {
		return "", false, nil
	}
	return strings.TrimSpace(entry.AnsibleHost), true, nil
}
