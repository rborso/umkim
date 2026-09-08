package sitecluster

import (
	"context"
	"fmt"
	"sort"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/clustercreate"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/nodeops"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitehosts"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProvisionedVMMinRootDiskGi matches typical root LV size after EFI/boot on a 40G vdisk.
const ProvisionedVMMinRootDiskGi = 38

func effectiveMinRootDiskGi(rec fleetv1alpha1.SiteHostRecord) int {
	if strings.TrimSpace(rec.Kind) == fleetv1alpha1.SiteHostKindVM {
		return ProvisionedVMMinRootDiskGi
	}
	return 0
}

// TryApplyHost attempts cluster handoff for one Ready host when intent is set.
func TryApplyHost(ctx context.Context, c client.Client, cfg config.Config, siteName string, hostName string) error {
	rec, ok, err := sitehosts.Get(ctx, c, cfg.Namespace, siteName, hostName)
	if err != nil || !ok {
		return err
	}
	return applyRecord(ctx, c, cfg, siteName, rec)
}

// ReconcileSite refreshes cluster status and applies pending intents for all hosts.
func ReconcileSite(ctx context.Context, c client.Client, cfg config.Config, siteName string) error {
	hosts, err := sitehosts.List(ctx, c, cfg.Namespace, siteName)
	if err != nil {
		return err
	}
	for _, h := range hosts {
		if err := syncClusterStatus(ctx, c, cfg.Namespace, siteName, h); err != nil {
			return err
		}
	}
	// Reload after sync: apply uses cluster phase to decide whether to re-enroll.
	hosts, err = sitehosts.List(ctx, c, cfg.Namespace, siteName)
	if err != nil {
		return err
	}
	for _, h := range hosts {
		if h.Status.Phase != fleetv1alpha1.SiteHostPhaseReady {
			continue
		}
		if h.ClusterIntent == nil {
			continue
		}
		if !shouldApplyIntent(h) {
			continue
		}
		if err := applyRecord(ctx, c, cfg, siteName, h); err != nil {
			_ = sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, h.Name, fleetv1alpha1.SiteHostClusterStatus{
				Phase:   fleetv1alpha1.SiteHostClusterPhaseFailed,
				Message: err.Error(),
				Mode:    h.ClusterIntent.Mode,
			})
		}
	}
	return nil
}

func shouldApplyIntent(rec fleetv1alpha1.SiteHostRecord) bool {
	if rec.ClusterIntent == nil || !rec.ClusterIntent.EffectiveAutoApply() {
		return false
	}
	if rec.ClusterIntent.Mode == "" {
		return false
	}
	st := rec.Status.Cluster
	if st == nil {
		return true
	}
	switch st.Phase {
	case fleetv1alpha1.SiteHostClusterPhaseReady, fleetv1alpha1.SiteHostClusterPhaseApplying:
		return false
	case fleetv1alpha1.SiteHostClusterPhaseWaitingGroup:
		return true
	case fleetv1alpha1.SiteHostClusterPhaseFailed:
		return true
	default:
		return st.Phase == "" || st.Phase == fleetv1alpha1.SiteHostClusterPhasePending
	}
}

func applyRecord(ctx context.Context, c client.Client, cfg config.Config, siteName string, rec fleetv1alpha1.SiteHostRecord) error {
	if rec.ClusterIntent == nil {
		return fmt.Errorf("cluster intent not set")
	}
	if rec.Status.Phase != fleetv1alpha1.SiteHostPhaseReady {
		return fmt.Errorf("host is not ready")
	}
	addr := hostAddress(rec)
	if addr == "" {
		return fmt.Errorf("host has no ansible_host")
	}
	intent := rec.ClusterIntent
	mode := strings.TrimSpace(intent.Mode)
	switch mode {
	case fleetv1alpha1.SiteHostClusterModeEnroll:
		return applyEnroll(ctx, c, cfg, siteName, rec, addr)
	case fleetv1alpha1.SiteHostClusterModeAddWorker:
		return applyAddWorker(ctx, c, cfg, siteName, rec, addr)
	case fleetv1alpha1.SiteHostClusterModeReplaceControl:
		return applyReplaceControl(ctx, c, cfg, siteName, rec, addr)
	default:
		return fmt.Errorf("unsupported cluster intent mode %q", mode)
	}
}

func applyEnroll(ctx context.Context, c client.Client, cfg config.Config, siteName string, rec fleetv1alpha1.SiteHostRecord, addr string) error {
	intent := rec.ClusterIntent
	topology := intent.EffectiveTopology()
	clusterName := strings.TrimSpace(intent.ClusterName)
	if clusterName == "" {
		clusterName = rec.Name + "-0"
	}
	if topology == fleetv1alpha1.ClusterTopologyHA3Node {
		return applyEnrollHA(ctx, c, cfg, siteName, rec, clusterName, intent)
	}
	if handled, err := syncHostFromExistingFCJ(ctx, c, cfg.Namespace, siteName, rec, intent, clusterName); handled || err != nil {
		return err
	}
	st := clusterStatusForIntent(intent, clusterName)
	st.Phase = fleetv1alpha1.SiteHostClusterPhaseApplying
	st.Message = "Creating FleetClusterJoin"
	if err := sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, rec.Name, st); err != nil {
		return err
	}
	_, err := clustercreate.Create(ctx, c, cfg, clustercreate.Request{
		ClusterName:     clusterName,
		SiteId:          siteName,
		BootstrapHost:   addr,
		Jumphost:        intent.Jumphost,
		MinRootDiskGi:   effectiveMinRootDiskGi(rec),
		Topology:        topology,
		ControlPlaneVip: intent.ControlPlaneVip,
		Edge: fleetv1alpha1.FleetClusterJoinEdgeSpec{
			IngressId: intent.IngressId,
		},
		Nodes: []fleetv1alpha1.FleetClusterJoinNodeSpec{{
			Hostname: rec.Name,
			Address:  addr,
			Role:     fleetv1alpha1.NodeRoleControl,
		}},
	})
	if err != nil {
		st.Phase = fleetv1alpha1.SiteHostClusterPhaseFailed
		st.Message = err.Error()
		_ = sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, rec.Name, st)
		return err
	}
	st.Message = "FleetClusterJoin created"
	return sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, rec.Name, st)
}

func applyEnrollHA(ctx context.Context, c client.Client, cfg config.Config, siteName string, rec fleetv1alpha1.SiteHostRecord, clusterName string, intent *fleetv1alpha1.SiteHostClusterIntent) error {
	if handled, err := syncHostFromExistingFCJ(ctx, c, cfg.Namespace, siteName, rec, intent, clusterName); handled || err != nil {
		return err
	}
	group := strings.TrimSpace(intent.HAGroup)
	if group == "" {
		group = clusterName
	}
	members, err := haGroupMembers(ctx, c, cfg.Namespace, siteName, group, clusterName, intent.EffectiveTopology())
	if err != nil {
		return err
	}
	if len(members) != 3 {
		st := clusterStatusForIntent(intent, clusterName)
		st.Phase = fleetv1alpha1.SiteHostClusterPhaseWaitingGroup
		st.Message = fmt.Sprintf("waiting for %d/3 HA group hosts Ready", len(members))
		return sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, rec.Name, st)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })
	if members[0].Name != rec.Name {
		st := clusterStatusForIntent(intent, clusterName)
		st.Phase = fleetv1alpha1.SiteHostClusterPhaseWaitingGroup
		st.Message = "waiting for HA group leader to create cluster"
		return sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, rec.Name, st)
	}
	nodes := make([]fleetv1alpha1.FleetClusterJoinNodeSpec, 0, 3)
	for _, m := range members {
		nodes = append(nodes, fleetv1alpha1.FleetClusterJoinNodeSpec{
			Hostname: m.Name,
			Address:  hostAddress(m),
			Role:     fleetv1alpha1.NodeRoleControl,
		})
	}
	st := clusterStatusForIntent(intent, clusterName)
	st.Phase = fleetv1alpha1.SiteHostClusterPhaseApplying
	st.Message = "Creating HA FleetClusterJoin"
	if err := sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, rec.Name, st); err != nil {
		return err
	}
	_, err = clustercreate.Create(ctx, c, cfg, clustercreate.Request{
		ClusterName:     clusterName,
		SiteId:          siteName,
		BootstrapHost:   nodes[0].Address,
		Jumphost:        intent.Jumphost,
		MinRootDiskGi:   effectiveMinRootDiskGi(members[0]),
		Topology:        fleetv1alpha1.ClusterTopologyHA3Node,
		ControlPlaneVip: intent.ControlPlaneVip,
		Edge: fleetv1alpha1.FleetClusterJoinEdgeSpec{
			IngressId: intent.IngressId,
		},
		Nodes: nodes,
	})
	if err != nil {
		st.Phase = fleetv1alpha1.SiteHostClusterPhaseFailed
		st.Message = err.Error()
		_ = sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, rec.Name, st)
		return err
	}
	for _, m := range members {
		mst := clusterStatusForIntent(intent, clusterName)
		mst.Phase = fleetv1alpha1.SiteHostClusterPhaseApplying
		mst.Message = "FleetClusterJoin created"
		_ = sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, m.Name, mst)
	}
	return nil
}

func haGroupMembers(ctx context.Context, c client.Client, ns, siteName, group, clusterName, topology string) ([]fleetv1alpha1.SiteHostRecord, error) {
	hosts, err := sitehosts.List(ctx, c, ns, siteName)
	if err != nil {
		return nil, err
	}
	var out []fleetv1alpha1.SiteHostRecord
	for _, h := range hosts {
		if h.ClusterIntent == nil {
			continue
		}
		if h.ClusterIntent.Mode != fleetv1alpha1.SiteHostClusterModeEnroll {
			continue
		}
		if h.ClusterIntent.EffectiveTopology() != topology {
			continue
		}
		g := strings.TrimSpace(h.ClusterIntent.HAGroup)
		if g == "" {
			g = strings.TrimSpace(h.ClusterIntent.ClusterName)
		}
		if g == "" {
			g = clusterName
		}
		if g != group {
			continue
		}
		if h.Status.Phase != fleetv1alpha1.SiteHostPhaseReady || hostAddress(h) == "" {
			continue
		}
		out = append(out, h)
	}
	return out, nil
}

func applyAddWorker(ctx context.Context, c client.Client, cfg config.Config, siteName string, rec fleetv1alpha1.SiteHostRecord, addr string) error {
	target := strings.TrimSpace(rec.ClusterIntent.TargetCluster)
	if target == "" {
		return fmt.Errorf("targetCluster is required for add_worker")
	}
	st := clusterStatusForIntent(rec.ClusterIntent, target)
	st.Phase = fleetv1alpha1.SiteHostClusterPhaseApplying
	st.Message = "Submitting add-worker operation"
	if err := sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, rec.Name, st); err != nil {
		return err
	}
	err := nodeops.SubmitRequest(ctx, c, cfg.Namespace, target, &nodeops.Request{
		Type:     fleetv1alpha1.NodeOpTypeAddWorker,
		Address:  addr,
		Hostname: rec.Name,
		Role:     fleetv1alpha1.NodeRoleWorker,
	})
	if err != nil {
		st.Phase = fleetv1alpha1.SiteHostClusterPhaseFailed
		st.Message = err.Error()
		_ = sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, rec.Name, st)
		return err
	}
	st.Message = "Add-worker operation queued"
	return sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, rec.Name, st)
}

func applyReplaceControl(ctx context.Context, c client.Client, cfg config.Config, siteName string, rec fleetv1alpha1.SiteHostRecord, addr string) error {
	intent := rec.ClusterIntent
	target := strings.TrimSpace(intent.TargetCluster)
	oldNode := strings.TrimSpace(intent.ReplaceNode)
	if target == "" || oldNode == "" {
		return fmt.Errorf("targetCluster and replaceNode are required for replace_control")
	}
	st := clusterStatusForIntent(intent, target)
	st.Phase = fleetv1alpha1.SiteHostClusterPhaseApplying
	st.Message = "Submitting control-plane replace operation"
	if err := sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, rec.Name, st); err != nil {
		return err
	}
	err := nodeops.SubmitRequest(ctx, c, cfg.Namespace, target, &nodeops.Request{
		Type:    fleetv1alpha1.NodeOpTypeReplaceControlPlane,
		OldNode: oldNode,
		NewNode: &nodeops.NewNode{
			Address:  addr,
			Hostname: rec.Name,
		},
	})
	if err != nil {
		st.Phase = fleetv1alpha1.SiteHostClusterPhaseFailed
		st.Message = err.Error()
		_ = sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, rec.Name, st)
		return err
	}
	st.Message = "Control-plane replace operation queued"
	return sitehosts.UpdateClusterStatus(ctx, c, cfg.Namespace, siteName, rec.Name, st)
}

func syncHostFromExistingFCJ(ctx context.Context, c client.Client, ns, siteName string, rec fleetv1alpha1.SiteHostRecord, intent *fleetv1alpha1.SiteHostClusterIntent, clusterName string) (bool, error) {
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: clusterName}, &fcj); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	st := clusterStatusFromFCJ(intent, clusterName, &fcj)
	if rec.Status.Cluster != nil && st.Phase == rec.Status.Cluster.Phase && st.Message == rec.Status.Cluster.Message {
		return true, nil
	}
	return true, sitehosts.UpdateClusterStatus(ctx, c, ns, siteName, rec.Name, st)
}

func clusterStatusFromFCJ(intent *fleetv1alpha1.SiteHostClusterIntent, clusterName string, fcj *fleetv1alpha1.FleetClusterJoin) fleetv1alpha1.SiteHostClusterStatus {
	st := fleetv1alpha1.SiteHostClusterStatus{
		ClusterName: clusterName,
		FCJName:     clusterName,
	}
	if intent != nil {
		st = clusterStatusForIntent(intent, clusterName)
	}
	switch fcj.Status.Phase {
	case fleetv1alpha1.JoinPhaseReady:
		st.Phase = fleetv1alpha1.SiteHostClusterPhaseReady
	case fleetv1alpha1.JoinPhaseFailed:
		st.Phase = fleetv1alpha1.SiteHostClusterPhaseFailed
	default:
		st.Phase = fleetv1alpha1.SiteHostClusterPhaseApplying
	}
	st.Message = fcj.Status.Message
	return st
}

func syncClusterStatus(ctx context.Context, c client.Client, ns, siteName string, rec fleetv1alpha1.SiteHostRecord) error {
	if rec.Status.Cluster == nil {
		return nil
	}
	fcjName := rec.Status.Cluster.FCJName
	if fcjName == "" {
		fcjName = rec.Status.Cluster.ClusterName
	}
	if fcjName == "" {
		return nil
	}
	fcj, err := nodeops.FindFleetClusterJoin(ctx, c, ns, fcjName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	st := clusterStatusFromFCJ(rec.ClusterIntent, fcjName, fcj)
	if rec.Status.Cluster != nil {
		st.Mode = rec.Status.Cluster.Mode
		if st.ClusterName == "" {
			st.ClusterName = rec.Status.Cluster.ClusterName
		}
		if st.FCJName == "" {
			st.FCJName = rec.Status.Cluster.FCJName
		}
	}
	if rec.ClusterIntent != nil && rec.ClusterIntent.Mode == fleetv1alpha1.SiteHostClusterModeAddWorker {
		if fcj.Status.NodeOperation != nil {
			switch fcj.Status.NodeOperation.Phase {
			case fleetv1alpha1.NodeOpPhaseSucceeded:
				st.Phase = fleetv1alpha1.SiteHostClusterPhaseReady
				st.Message = fcj.Status.NodeOperation.Message
			case fleetv1alpha1.NodeOpPhaseFailed:
				st.Phase = fleetv1alpha1.SiteHostClusterPhaseFailed
				st.Message = fcj.Status.NodeOperation.Message
			case fleetv1alpha1.NodeOpPhaseRunning, fleetv1alpha1.NodeOpPhasePending:
				st.Phase = fleetv1alpha1.SiteHostClusterPhaseApplying
				st.Message = fcj.Status.NodeOperation.Message
			}
		}
	}
	if rec.ClusterIntent != nil && rec.ClusterIntent.Mode == fleetv1alpha1.SiteHostClusterModeReplaceControl {
		if fcj.Status.NodeOperation != nil {
			switch fcj.Status.NodeOperation.Phase {
			case fleetv1alpha1.NodeOpPhaseSucceeded:
				st.Phase = fleetv1alpha1.SiteHostClusterPhaseReady
				st.Message = fcj.Status.NodeOperation.Message
			case fleetv1alpha1.NodeOpPhaseFailed:
				st.Phase = fleetv1alpha1.SiteHostClusterPhaseFailed
				st.Message = fcj.Status.NodeOperation.Message
			case fleetv1alpha1.NodeOpPhaseRunning, fleetv1alpha1.NodeOpPhasePending:
				st.Phase = fleetv1alpha1.SiteHostClusterPhaseApplying
				st.Message = fcj.Status.NodeOperation.Message
			}
		}
	}
	if st.Phase == rec.Status.Cluster.Phase && st.Message == rec.Status.Cluster.Message {
		return nil
	}
	return sitehosts.UpdateClusterStatus(ctx, c, ns, siteName, rec.Name, st)
}

func clusterStatusForIntent(intent *fleetv1alpha1.SiteHostClusterIntent, clusterName string) fleetv1alpha1.SiteHostClusterStatus {
	name := clusterName
	if intent != nil && strings.TrimSpace(intent.TargetCluster) != "" {
		name = strings.TrimSpace(intent.TargetCluster)
	}
	return fleetv1alpha1.SiteHostClusterStatus{
		Mode:        intent.Mode,
		ClusterName: name,
		FCJName:     name,
	}
}

func hostAddress(rec fleetv1alpha1.SiteHostRecord) string {
	if v := strings.TrimSpace(rec.AnsibleHost); v != "" {
		return v
	}
	if rec.Status.AnsibleHost != "" {
		return strings.TrimSpace(rec.Status.AnsibleHost)
	}
	return ""
}

// ApplyHostNow applies cluster intent for a Ready host (manual trigger).
func ApplyHostNow(ctx context.Context, c client.Client, cfg config.Config, siteName, hostName string) error {
	rec, ok, err := sitehosts.Get(ctx, c, cfg.Namespace, siteName, hostName)
	if err != nil || !ok {
		return fmt.Errorf("host not found")
	}
	if rec.ClusterIntent == nil {
		return fmt.Errorf("cluster intent not set")
	}
	if rec.Status.Phase != fleetv1alpha1.SiteHostPhaseReady {
		return fmt.Errorf("host is not ready")
	}
	return applyRecord(ctx, c, cfg, siteName, rec)
}
