package sitecluster

import (
	"context"
	"fmt"
	"sort"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/clustercreate"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/nodeops"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sites"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// inventoryStoreName is the FleetSite metadata name, which keys the inventory ConfigMap.
// siteName may be that name or spec.siteId.
func inventoryStoreName(ctx context.Context, c client.Client, ns, siteName string) string {
	fs, err := sites.FindFleetSite(ctx, c, ns, siteName)
	if err != nil || fs == nil {
		return siteName
	}
	return fs.Name
}

func inventoryJoinSiteID(ctx context.Context, c client.Client, ns, storeName string) string {
	fs, err := sites.FindFleetSite(ctx, c, ns, storeName)
	if err != nil || fs == nil {
		return storeName
	}
	return fs.Spec.EffectiveSiteID(fs.Name)
}

// ReconcileInventorySite applies cluster intents from root group + group_vars files for inventory-first sites.
func ReconcileInventorySite(ctx context.Context, c client.Client, cfg config.Config, siteName string) error {
	siteName = inventoryStoreName(ctx, c, cfg.Namespace, siteName)
	clustersDoc, err := yaidap.LoadClustersFile(ctx, c, cfg.Namespace, siteName)
	if err != nil {
		return err
	}
	hosts, err := yaidap.ListInventoryHosts(ctx, c, cfg.Namespace, siteName)
	if err != nil {
		return err
	}
	hostByName := map[string]yaidap.InventoryHostView{}
	for _, h := range hosts {
		hostByName[h.Name] = h
		if err := syncInventoryGuestClusterStatus(ctx, c, cfg.Namespace, siteName, h); err != nil {
			return err
		}
	}
	for clusterName, spec := range clustersDoc.Clusters {
		if !spec.EffectiveAutoApply() {
			continue
		}
		if err := applyClusterSpec(ctx, c, cfg, siteName, clusterName, spec, hostByName); err != nil {
			for _, m := range spec.Members {
				_ = yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, m, yaidap.GuestStatus{
					Cluster: &fleetv1alpha1.SiteHostClusterStatus{
						Phase:       fleetv1alpha1.SiteHostClusterPhaseFailed,
						Message:     err.Error(),
						ClusterName: clusterName,
					},
				})
			}
		}
	}
	return nil
}

func applyClusterSpec(ctx context.Context, c client.Client, cfg config.Config, siteName, clusterName string, spec yaidap.ClusterSpec, hostByName map[string]yaidap.InventoryHostView) error {
	intent := yaidap.ClusterIntentFromSpec(clusterName, spec)
	mode := intent.Mode
	switch mode {
	case fleetv1alpha1.SiteHostClusterModeEnroll:
		return applyInventoryEnroll(ctx, c, cfg, siteName, clusterName, spec, intent, hostByName)
	case fleetv1alpha1.SiteHostClusterModeAddWorker:
		if len(spec.Members) != 1 {
			return fmt.Errorf("add_worker cluster %q requires one member", clusterName)
		}
		return applyInventoryAddWorker(ctx, c, cfg, siteName, spec.Members[0], intent, hostByName)
	case fleetv1alpha1.SiteHostClusterModeReplaceControl:
		if len(spec.Members) != 1 {
			return fmt.Errorf("replace_control cluster %q requires one member", clusterName)
		}
		return applyInventoryReplaceControl(ctx, c, cfg, siteName, spec.Members[0], intent, hostByName)
	default:
		return fmt.Errorf("unsupported cluster mode %q", mode)
	}
}

func applyInventoryEnroll(ctx context.Context, c client.Client, cfg config.Config, siteName, clusterName string, spec yaidap.ClusterSpec, intent *fleetv1alpha1.SiteHostClusterIntent, hostByName map[string]yaidap.InventoryHostView) error {
	topology := spec.EffectiveTopology()
	if topology == fleetv1alpha1.ClusterTopologyHA3Node {
		return applyInventoryEnrollHA(ctx, c, cfg, siteName, clusterName, spec, intent, hostByName)
	}
	if len(spec.Members) != 1 {
		return fmt.Errorf("single_node cluster %q requires one member", clusterName)
	}
	member := spec.Members[0]
	h, ok := hostByName[member]
	if !ok {
		return fmt.Errorf("member %q not in inventory", member)
	}
	if !guestReady(h) {
		return nil
	}
	addr := strings.TrimSpace(h.AnsibleHost)
	if addr == "" {
		return fmt.Errorf("member %q has no ansible_host", member)
	}
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := c.Get(ctx, client.ObjectKey{Namespace: cfg.Namespace, Name: clusterName}, &fcj); err == nil {
		st := clusterStatusFromFCJ(intent, clusterName, &fcj)
		return yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, member, yaidap.GuestStatus{Cluster: &st})
	}
	st := clusterStatusForIntent(intent, clusterName)
	st.Phase = fleetv1alpha1.SiteHostClusterPhaseApplying
	st.Message = "Creating FleetClusterJoin"
	_ = yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, member, yaidap.GuestStatus{Cluster: &st})
	_, err := clustercreate.Create(ctx, c, cfg, clustercreate.Request{
		ClusterName:     clusterName,
		SiteId:          inventoryJoinSiteID(ctx, c, cfg.Namespace, siteName),
		BootstrapHost:   addr,
		Jumphost:        intent.Jumphost,
		MinRootDiskGi:   ProvisionedVMMinRootDiskGi,
		Topology:        topology,
		ControlPlaneVip: intent.ControlPlaneVip,
		Edge:            fleetv1alpha1.FleetClusterJoinEdgeSpec{IngressId: intent.IngressId},
		Nodes: []fleetv1alpha1.FleetClusterJoinNodeSpec{{
			Hostname: member,
			Address:  addr,
			Role:     fleetv1alpha1.NodeRoleControl,
		}},
	})
	if err != nil {
		st.Phase = fleetv1alpha1.SiteHostClusterPhaseFailed
		st.Message = err.Error()
		_ = yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, member, yaidap.GuestStatus{Cluster: &st})
		return err
	}
	st.Message = "FleetClusterJoin created"
	return yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, member, yaidap.GuestStatus{Cluster: &st})
}

func applyInventoryEnrollHA(ctx context.Context, c client.Client, cfg config.Config, siteName, clusterName string, spec yaidap.ClusterSpec, intent *fleetv1alpha1.SiteHostClusterIntent, hostByName map[string]yaidap.InventoryHostView) error {
	var members []yaidap.InventoryHostView
	for _, name := range spec.Members {
		h, ok := hostByName[name]
		if !ok {
			return fmt.Errorf("member %q not in inventory", name)
		}
		if !guestReady(h) {
			st := clusterStatusForIntent(intent, clusterName)
			st.Phase = fleetv1alpha1.SiteHostClusterPhaseWaitingGroup
			st.Message = fmt.Sprintf("waiting for %d/%d HA members Ready", len(members), len(spec.Members))
			_ = yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, name, yaidap.GuestStatus{Cluster: &st})
			return nil
		}
		if strings.TrimSpace(h.AnsibleHost) == "" {
			return fmt.Errorf("member %q has no ansible_host", name)
		}
		members = append(members, h)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })
	leader := members[0]
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := c.Get(ctx, client.ObjectKey{Namespace: cfg.Namespace, Name: clusterName}, &fcj); err == nil {
		st := clusterStatusFromFCJ(intent, clusterName, &fcj)
		for _, m := range members {
			_ = yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, m.Name, yaidap.GuestStatus{Cluster: &st})
		}
		return nil
	}
	if leader.Name != spec.Members[0] && !containsName(spec.Members, leader.Name) {
		// leader is lexicographically first ready member
	}
	nodes := make([]fleetv1alpha1.FleetClusterJoinNodeSpec, 0, len(members))
	for _, m := range members {
		nodes = append(nodes, fleetv1alpha1.FleetClusterJoinNodeSpec{
			Hostname: m.Name,
			Address:  m.AnsibleHost,
			Role:     fleetv1alpha1.NodeRoleControl,
		})
	}
	st := clusterStatusForIntent(intent, clusterName)
	st.Phase = fleetv1alpha1.SiteHostClusterPhaseApplying
	st.Message = "Creating HA FleetClusterJoin"
	for _, m := range members {
		_ = yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, m.Name, yaidap.GuestStatus{Cluster: &st})
	}
	_, err := clustercreate.Create(ctx, c, cfg, clustercreate.Request{
		ClusterName:     clusterName,
		SiteId:          inventoryJoinSiteID(ctx, c, cfg.Namespace, siteName),
		BootstrapHost:   leader.AnsibleHost,
		Jumphost:        intent.Jumphost,
		MinRootDiskGi:   ProvisionedVMMinRootDiskGi,
		Topology:        fleetv1alpha1.ClusterTopologyHA3Node,
		ControlPlaneVip: intent.ControlPlaneVip,
		Edge:            fleetv1alpha1.FleetClusterJoinEdgeSpec{IngressId: intent.IngressId},
		Nodes:           nodes,
	})
	if err != nil {
		st.Phase = fleetv1alpha1.SiteHostClusterPhaseFailed
		st.Message = err.Error()
		for _, m := range members {
			_ = yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, m.Name, yaidap.GuestStatus{Cluster: &st})
		}
		return err
	}
	st.Message = "FleetClusterJoin created"
	for _, m := range members {
		_ = yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, m.Name, yaidap.GuestStatus{Cluster: &st})
	}
	return nil
}

func applyInventoryAddWorker(ctx context.Context, c client.Client, cfg config.Config, siteName, member string, intent *fleetv1alpha1.SiteHostClusterIntent, hostByName map[string]yaidap.InventoryHostView) error {
	h, ok := hostByName[member]
	if !ok || !guestReady(h) {
		return nil
	}
	target := strings.TrimSpace(intent.TargetCluster)
	if target == "" {
		return fmt.Errorf("targetCluster required")
	}
	addr := strings.TrimSpace(h.AnsibleHost)
	st := clusterStatusForIntent(intent, target)
	st.Phase = fleetv1alpha1.SiteHostClusterPhaseApplying
	st.Message = "Submitting add-worker operation"
	_ = yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, member, yaidap.GuestStatus{Cluster: &st})
	err := nodeops.SubmitRequest(ctx, c, cfg.Namespace, target, &nodeops.Request{
		Type:     fleetv1alpha1.NodeOpTypeAddWorker,
		Address:  addr,
		Hostname: member,
		Role:     fleetv1alpha1.NodeRoleWorker,
	})
	if err != nil {
		st.Phase = fleetv1alpha1.SiteHostClusterPhaseFailed
		st.Message = err.Error()
		_ = yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, member, yaidap.GuestStatus{Cluster: &st})
		return err
	}
	st.Message = "Add-worker operation queued"
	return yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, member, yaidap.GuestStatus{Cluster: &st})
}

func applyInventoryReplaceControl(ctx context.Context, c client.Client, cfg config.Config, siteName, member string, intent *fleetv1alpha1.SiteHostClusterIntent, hostByName map[string]yaidap.InventoryHostView) error {
	h, ok := hostByName[member]
	if !ok || !guestReady(h) {
		return nil
	}
	target := strings.TrimSpace(intent.TargetCluster)
	oldNode := strings.TrimSpace(intent.ReplaceNode)
	if target == "" || oldNode == "" {
		return fmt.Errorf("targetCluster and replaceNode required")
	}
	addr := strings.TrimSpace(h.AnsibleHost)
	st := clusterStatusForIntent(intent, target)
	st.Phase = fleetv1alpha1.SiteHostClusterPhaseApplying
	st.Message = "Submitting control-plane replace operation"
	_ = yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, member, yaidap.GuestStatus{Cluster: &st})
	err := nodeops.SubmitRequest(ctx, c, cfg.Namespace, target, &nodeops.Request{
		Type:    fleetv1alpha1.NodeOpTypeReplaceControlPlane,
		OldNode: oldNode,
		NewNode: &nodeops.NewNode{Address: addr, Hostname: member},
	})
	if err != nil {
		st.Phase = fleetv1alpha1.SiteHostClusterPhaseFailed
		st.Message = err.Error()
		_ = yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, member, yaidap.GuestStatus{Cluster: &st})
		return err
	}
	st.Message = "Control-plane replace operation queued"
	return yaidap.UpdateGuestStatus(ctx, c, cfg.Namespace, siteName, member, yaidap.GuestStatus{Cluster: &st})
}

func syncInventoryGuestClusterStatus(ctx context.Context, c client.Client, ns, siteName string, h yaidap.InventoryHostView) error {
	rawDoc, err := yaidap.LoadStatusDoc(ctx, c, ns, siteName)
	if err != nil {
		return err
	}
	rawCluster := (*fleetv1alpha1.SiteHostClusterStatus)(nil)
	if g, ok := rawDoc.Guests[h.Name]; ok {
		rawCluster = g.Cluster
	}
	if yaidap.GuestClusterStatusNeedsFCJEnrich(rawCluster) {
		fcjSt, ok := yaidap.FCJClusterStatusForHost(ctx, c, ns, siteName, h.Name)
		if !ok {
			return nil
		}
		return yaidap.UpdateGuestStatus(ctx, c, ns, siteName, h.Name, yaidap.GuestStatus{Cluster: fcjSt})
	}
	if rawCluster == nil || rawCluster.FCJName == "" && rawCluster.ClusterName == "" {
		return nil
	}
	fcjName := rawCluster.FCJName
	if fcjName == "" {
		fcjName = rawCluster.ClusterName
	}
	fcj, err := nodeops.FindFleetClusterJoin(ctx, c, ns, fcjName)
	if err != nil {
		if nodeops.IsClusterNotFound(err) {
			return yaidap.UpdateGuestStatus(ctx, c, ns, siteName, h.Name, yaidap.GuestStatus{
				Cluster: &fleetv1alpha1.SiteHostClusterStatus{},
			})
		}
		return err
	}
	st := clusterStatusFromFCJ(nil, fcjName, fcj)
	if rawCluster != nil {
		st.Mode = rawCluster.Mode
	}
	if st.Phase == rawCluster.Phase && st.Message == rawCluster.Message {
		return nil
	}
	return yaidap.UpdateGuestStatus(ctx, c, ns, siteName, h.Name, yaidap.GuestStatus{Cluster: &st})
}

func guestReady(h yaidap.InventoryHostView) bool {
	return h.Status.Phase == fleetv1alpha1.SiteHostPhaseReady && strings.TrimSpace(h.AnsibleHost) != ""
}

func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// ApplyInventoryClusterNow manually applies one cluster from clusters.yml.
func ApplyInventoryClusterNow(ctx context.Context, c client.Client, cfg config.Config, siteName, clusterName string) error {
	siteName = inventoryStoreName(ctx, c, cfg.Namespace, siteName)
	doc, err := yaidap.LoadClustersFile(ctx, c, cfg.Namespace, siteName)
	if err != nil {
		return err
	}
	spec, ok := doc.Clusters[clusterName]
	if !ok {
		return fmt.Errorf("cluster %q not found in inventory group files", clusterName)
	}
	hosts, err := yaidap.ListInventoryHosts(ctx, c, cfg.Namespace, siteName)
	if err != nil {
		return err
	}
	hostByName := map[string]yaidap.InventoryHostView{}
	for _, h := range hosts {
		hostByName[h.Name] = h
	}
	return applyClusterSpec(ctx, c, cfg, siteName, clusterName, spec, hostByName)
}

// ApplyInventoryGuestCluster applies cluster intent for a guest listed in clusters.yml.
func ApplyInventoryGuestCluster(ctx context.Context, c client.Client, cfg config.Config, siteName, guestName string) error {
	siteName = inventoryStoreName(ctx, c, cfg.Namespace, siteName)
	doc, err := yaidap.LoadClustersFile(ctx, c, cfg.Namespace, siteName)
	if err != nil {
		return err
	}
	for clusterName, spec := range doc.Clusters {
		for _, m := range spec.Members {
			if m == guestName {
				hosts, err := yaidap.ListInventoryHosts(ctx, c, cfg.Namespace, siteName)
				if err != nil {
					return err
				}
				hostByName := map[string]yaidap.InventoryHostView{}
				for _, h := range hosts {
					hostByName[h.Name] = h
				}
				return applyClusterSpec(ctx, c, cfg, siteName, clusterName, spec, hostByName)
			}
		}
	}
	return fmt.Errorf("guest %q is not a member of any inventory cluster group", guestName)
}
