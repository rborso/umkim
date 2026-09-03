package reconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ansiblelogs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/nodeops"
	"github.com/rborso/umkim/controllers/fleet-site/internal/spoke"
	"github.com/rborso/umkim/controllers/fleet-site/internal/wg"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const annotationNodeOperationRequest = "fleet.umkim.com/node-operation-request"

type nodeOperationRequest struct {
	Type     string            `json:"type"`
	Node     string            `json:"node,omitempty"`
	Address  string            `json:"address,omitempty"`
	Hostname string            `json:"hostname,omitempty"`
	Role     string            `json:"role,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	OldNode    string            `json:"oldNode,omitempty"`
	NewNode    *nodeOpNewNode    `json:"newNode,omitempty"`
	Force      bool              `json:"force,omitempty"`
	ActorKind  string            `json:"actorKind,omitempty"`
	ActorName  string            `json:"actorName,omitempty"`
}

type nodeOpNewNode struct {
	Address  string            `json:"address"`
	Hostname string            `json:"hostname,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

func (r *ClusterJoinReconciler) reconcileNodeOperation(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		return ctrl.Result{}, nil
	}
	if fcj.Annotations != nil {
		if raw := strings.TrimSpace(fcj.Annotations[annotationNodeOperationRequest]); raw != "" {
			if fcj.Status.NodeOperation != nil && fcj.Status.NodeOperation.Phase == fleetv1alpha1.NodeOpPhaseRunning {
				return ctrl.Result{}, nil
			}
			var req nodeOperationRequest
			if err := json.Unmarshal([]byte(raw), &req); err != nil {
				return r.failNodeOperation(ctx, fcj, fmt.Errorf("invalid node operation request: %w", err))
			}
			if err := r.acceptNodeOperationRequest(ctx, fcj, clusterName, &req); err != nil {
				return r.failNodeOperation(ctx, fcj, err)
			}
			return ctrl.Result{Requeue: true}, nil
		}
	}
	op := fcj.Status.NodeOperation
	if op == nil {
		return ctrl.Result{}, nil
	}
	switch op.Phase {
	case fleetv1alpha1.NodeOpPhasePending:
		return r.startNodeOperationJob(ctx, fcj, clusterName, op)
	case fleetv1alpha1.NodeOpPhaseRunning:
		return r.pollNodeOperationJob(ctx, fcj, clusterName)
	case fleetv1alpha1.NodeOpPhaseSucceeded, fleetv1alpha1.NodeOpPhaseFailed:
		return ctrl.Result{}, nil
	default:
		return ctrl.Result{}, nil
	}
}

func (r *ClusterJoinReconciler) acceptNodeOperationRequest(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string, req *nodeOperationRequest) error {
	if req.Type == "" {
		return fmt.Errorf("node operation type is required")
	}
	node := strings.TrimSpace(req.Node)
	switch req.Type {
	case fleetv1alpha1.NodeOpTypeAddWorker:
		if strings.TrimSpace(req.Address) == "" {
			return fmt.Errorf("address is required to add a worker")
		}
		if req.Hostname == "" {
			req.Hostname = nextWorkerHostname(fcj, clusterName)
		}
		node = req.Hostname
		if err := r.ensureAddWorkerSpec(ctx, fcj, clusterName, req); err != nil {
			return err
		}
	case fleetv1alpha1.NodeOpTypeRemoveWorker:
		if node == "" {
			return fmt.Errorf("node name is required to remove a worker")
		}
	case fleetv1alpha1.NodeOpTypeReplaceControlPlane:
		if strings.TrimSpace(req.OldNode) == "" {
			return fmt.Errorf("oldNode is required to replace control plane")
		}
		if req.NewNode == nil || strings.TrimSpace(req.NewNode.Address) == "" {
			return fmt.Errorf("newNode.address is required to replace control plane")
		}
		node = req.OldNode
		if err := r.ensureReplaceControlSpec(ctx, fcj, clusterName, req); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported node operation type %q", req.Type)
	}
	now := metav1.Now()
	key := client.ObjectKeyFromObject(fcj)
	opMessage := "Node operation queued"
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		if latest.Status.NodeOperation != nil && latest.Status.NodeOperation.Phase == fleetv1alpha1.NodeOpPhaseRunning {
			return fmt.Errorf("node operation already running")
		}
		if latest.Annotations != nil {
			delete(latest.Annotations, annotationNodeOperationRequest)
			if err := r.Update(ctx, &latest); err != nil {
				return err
			}
		}
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		latest.Status.NodeOperation = &fleetv1alpha1.ClusterNodeOperationStatus{
			ID:        fmt.Sprintf("nop-%d", now.UnixNano()),
			Type:      req.Type,
			Node:      node,
			Phase:     fleetv1alpha1.NodeOpPhasePending,
			Message:   opMessage,
			Force:     req.Force,
			ActorKind: strings.TrimSpace(req.ActorKind),
			ActorName: strings.TrimSpace(req.ActorName),
			StartedAt: &now,
		}
		latest.Status.Message = opMessage
		return r.Status().Update(ctx, &latest)
	})
}

func nextWorkerHostname(fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) string {
	max := 0
	for _, n := range fcj.Spec.Nodes {
		if n.EffectiveRole() != fleetv1alpha1.NodeRoleWorker {
			continue
		}
		hn := n.Hostname
		if strings.HasPrefix(hn, clusterName+"-w") {
			var idx int
			if _, err := fmt.Sscanf(hn, clusterName+"-w%d", &idx); err == nil && idx > max {
				max = idx
			}
		}
	}
	return fmt.Sprintf("%s-w%d", clusterName, max+1)
}

func (r *ClusterJoinReconciler) startNodeOperationJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string, op *fleetv1alpha1.ClusterNodeOperationStatus) (ctrl.Result, error) {
	nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
	if err != nil {
		return r.failNodeOperation(ctx, fcj, err)
	}
	extraArgs, targetHost, err := r.prepareNodeOperation(ctx, fcj, clusterName, op, nodeWG)
	if err != nil {
		return r.failNodeOperation(ctx, fcj, err)
	}
	nodeWG, err = r.clusterNodeWG(ctx, fcj, clusterName)
	if err != nil {
		return r.failNodeOperation(ctx, fcj, err)
	}
	if err := r.ensureClusterInventoryCM(ctx, fcj, clusterName, nodeWG); err != nil {
		return r.failNodeOperation(ctx, fcj, err)
	}
	if _, err := r.ensureClusterSSHSecret(ctx, fcj, clusterName); err != nil {
		return r.failNodeOperation(ctx, fcj, err)
	}
	jobName := fmt.Sprintf("fleet-cluster-nodeop-%s-%d", clusterName, time.Now().Unix())
	var j *batchv1.Job
	switch op.Type {
	case fleetv1alpha1.NodeOpTypeAddWorker:
		j = jobs.AddWorkerJob(fcj.Namespace, jobName, clusterName, fcj.Spec.SiteId, targetHost,
			r.Cfg.AnsibleRunnerImage, clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName), extraArgs)
	case fleetv1alpha1.NodeOpTypeRemoveWorker:
		j = jobs.RemoveWorkerJob(fcj.Namespace, jobName, clusterName, fcj.Spec.SiteId,
			r.Cfg.AnsibleRunnerImage, clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName), extraArgs)
	case fleetv1alpha1.NodeOpTypeReplaceControlPlane:
		j = jobs.ReplaceControlJob(fcj.Namespace, jobName, clusterName, fcj.Spec.SiteId,
			r.Cfg.AnsibleRunnerImage, clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName), extraArgs)
	default:
		return r.failNodeOperation(ctx, fcj, fmt.Errorf("unsupported node operation %q", op.Type))
	}
	if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Create(ctx, j); err != nil && !apierrors.IsAlreadyExists(err) {
		return r.failNodeOperation(ctx, fcj, err)
	}
	key := client.ObjectKeyFromObject(fcj)
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		if latest.Status.NodeOperation == nil {
			latest.Status.NodeOperation = &fleetv1alpha1.ClusterNodeOperationStatus{}
		}
		latest.Status.NodeOperation.Phase = fleetv1alpha1.NodeOpPhaseRunning
		latest.Status.NodeOperation.JobName = jobName
		latest.Status.NodeOperation.Message = nodeOperationRunningMessage(op.Type)
		latest.Status.Message = latest.Status.NodeOperation.Message
		return r.Status().Update(ctx, &latest)
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func nodeOperationRunningMessage(opType string) string {
	switch opType {
	case fleetv1alpha1.NodeOpTypeAddWorker:
		return "Adding worker node (Ansible)"
	case fleetv1alpha1.NodeOpTypeRemoveWorker:
		return "Removing worker node (Ansible)"
	case fleetv1alpha1.NodeOpTypeReplaceControlPlane:
		return "Replacing control plane node (Ansible)"
	default:
		return "Running node operation (Ansible)"
	}
}

func (r *ClusterJoinReconciler) prepareNodeOperation(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string, op *fleetv1alpha1.ClusterNodeOperationStatus, nodeWG []wg.NodeWG) ([]string, string, error) {
	req, err := r.nodeOperationRequestFromStatus(fcj, op)
	if err != nil {
		return nil, "", err
	}
	extra := []string{
		"-e", "fleet_node_operation_target=" + op.Node,
	}
	targetHost := op.Node
	switch op.Type {
	case fleetv1alpha1.NodeOpTypeAddWorker:
		if err := r.syncNodeWGForSpec(ctx, fcj, clusterName); err != nil {
			return nil, "", err
		}
		targetHost = req.Hostname
		extra = []string{"-e", "fleet_node_operation_target=" + targetHost}
		extra = append(extra, r.wgExtraArgsForTarget(ctx, fcj, clusterName, targetHost)...)
	case fleetv1alpha1.NodeOpTypeRemoveWorker:
		if op.Force {
			extra = append(extra, "-e", "fleet_node_remove_force=true")
		}
	case fleetv1alpha1.NodeOpTypeReplaceControlPlane:
		if err := r.syncNodeWGForSpec(ctx, fcj, clusterName); err != nil {
			return nil, "", err
		}
		if req.NewNode != nil && req.NewNode.Hostname != "" {
			extra = append(extra, "-e", "fleet_node_replace_new_hostname="+req.NewNode.Hostname)
		}
		if req.Force {
			extra = append(extra, "-e", "fleet_node_replace_force=true")
		}
		extra = append(extra, "-e", "fleet_node_replace_old="+req.OldNode)
		newHost := req.OldNode
		if req.NewNode != nil && req.NewNode.Hostname != "" {
			newHost = req.NewNode.Hostname
		}
		extra = append(extra, r.wgExtraArgsForTarget(ctx, fcj, clusterName, newHost)...)
	}
	return extra, targetHost, nil
}

func (r *ClusterJoinReconciler) wgExtraArgsForTarget(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName, targetHost string) []string {
	out := []string{}
	if targetHost == "" {
		return out
	}
	nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
	if err != nil {
		return out
	}
	for _, n := range nodeWG {
		if n.Hostname != targetHost {
			continue
		}
		if n.PrivateKey != "" {
			out = append(out, "-e", "fleet_node_wg_private_key="+n.PrivateKey)
		}
		if n.PublicKey != "" {
			out = append(out, "-e", "fleet_node_wg_public_key="+n.PublicKey)
		}
		break
	}
	hubPub := r.hubPublicKey(ctx)
	if hubPub != "" {
		out = append(out, "-e", "fleet_hub_wg_public_key="+hubPub)
	}
	out = append(out, "-e", "wireguard_subnet="+r.Cfg.WireguardSubnet)
	return out
}

func (r *ClusterJoinReconciler) nodeOperationRequestFromStatus(fcj *fleetv1alpha1.FleetClusterJoin, op *fleetv1alpha1.ClusterNodeOperationStatus) (*nodeOperationRequest, error) {
	req := &nodeOperationRequest{
		Type:  op.Type,
		Node:  op.Node,
		Force: op.Force,
	}
	switch op.Type {
	case fleetv1alpha1.NodeOpTypeAddWorker:
		spec, ok := findNodeSpec(fcj, op.Node)
		if !ok {
			return nil, fmt.Errorf("worker %s not found in cluster spec", op.Node)
		}
		req.Address = spec.Address
		req.Hostname = spec.Hostname
		req.Labels = spec.Labels
	case fleetv1alpha1.NodeOpTypeReplaceControlPlane:
		req.OldNode = op.Node
		for _, n := range fcj.Spec.Nodes {
			if n.EffectiveRole() != fleetv1alpha1.NodeRoleControl {
				continue
			}
			if n.Hostname != op.Node && n.Address != "" {
				req.NewNode = &nodeOpNewNode{
					Address:  n.Address,
					Hostname: n.Hostname,
					Labels:   n.Labels,
				}
				break
			}
		}
		if req.NewNode == nil {
			return nil, fmt.Errorf("replacement control node not found in spec")
		}
	}
	return req, nil
}

func findNodeSpec(fcj *fleetv1alpha1.FleetClusterJoin, hostname string) (fleetv1alpha1.FleetClusterJoinNodeSpec, bool) {
	for _, n := range fcj.Spec.Nodes {
		hn := n.Hostname
		if hn == "" {
			continue
		}
		if hn == hostname {
			return n, true
		}
	}
	return fleetv1alpha1.FleetClusterJoinNodeSpec{}, false
}

func (r *ClusterJoinReconciler) ensureAddWorkerSpec(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string, req *nodeOperationRequest) error {
	if req.Hostname == "" {
		req.Hostname = nextWorkerHostname(fcj, clusterName)
	}
	if _, ok := findNodeSpec(fcj, req.Hostname); ok {
		return nil
	}
	key := client.ObjectKeyFromObject(fcj)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		if _, ok := findNodeSpec(&latest, req.Hostname); ok {
			return nil
		}
		latest.Spec.Nodes = append(latest.Spec.Nodes, fleetv1alpha1.FleetClusterJoinNodeSpec{
			Hostname: req.Hostname,
			Address:  req.Address,
			Role:     fleetv1alpha1.NodeRoleWorker,
			Labels:   req.Labels,
		})
		return r.Update(ctx, &latest)
	})
}

func (r *ClusterJoinReconciler) ensureReplaceControlSpec(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string, req *nodeOperationRequest) error {
	if req.NewNode == nil {
		return fmt.Errorf("newNode is required")
	}
	newHN := strings.TrimSpace(req.NewNode.Hostname)
	if newHN == "" {
		newHN = req.OldNode
	}
	key := client.ObjectKeyFromObject(fcj)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		nodes := make([]fleetv1alpha1.FleetClusterJoinNodeSpec, 0, len(latest.Spec.Nodes)+1)
		replaced := false
		for _, n := range latest.Spec.Nodes {
			hn := n.Hostname
			if hn == req.OldNode {
				nodes = append(nodes, fleetv1alpha1.FleetClusterJoinNodeSpec{
					Hostname: newHN,
					Address:  req.NewNode.Address,
					Role:     fleetv1alpha1.NodeRoleControl,
					Labels:   req.NewNode.Labels,
				})
				replaced = true
				continue
			}
			nodes = append(nodes, n)
		}
		if !replaced {
			nodes = append(nodes, fleetv1alpha1.FleetClusterJoinNodeSpec{
				Hostname: newHN,
				Address:  req.NewNode.Address,
				Role:     fleetv1alpha1.NodeRoleControl,
			})
		}
		latest.Spec.Nodes = nodes
		return r.Update(ctx, &latest)
	})
}

func (r *ClusterJoinReconciler) syncNodeWGForSpec(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) error {
	key := client.ObjectKeyFromObject(fcj)
	var latest fleetv1alpha1.FleetClusterJoin
	if err := r.Get(ctx, key, &latest); err != nil {
		return err
	}
	used, err := r.usedOverlayIPs(ctx)
	if err != nil {
		return err
	}
	var secret corev1.Secret
	existing := []wg.NodeWG{}
	secretName := clusterBootstrapSecretName(clusterName)
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: latest.Namespace}, &secret); err == nil {
		existing, _ = wg.NodeWGFromJSON(secret.Data["nodes-wg.json"])
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	nodeWG, err := wg.AllocateClusterNodes(&latest, clusterName, used, r.Cfg.PoolStart, r.Cfg.PoolEnd, existing)
	if err != nil {
		return err
	}
	nodesWGJSON, err := wg.NodeWGToJSON(nodeWG)
	if err != nil {
		return err
	}
	if secret.Name != "" {
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Data["nodes-wg.json"] = nodesWGJSON
		if err := r.Update(ctx, &secret); err != nil {
			return err
		}
	}
	wgPeers := make([]jobs.WGPeer, 0, len(nodeWG))
	newPubkeys := map[string]struct{}{}
	for _, n := range nodeWG {
		wgPeers = append(wgPeers, jobs.WGPeer{PublicKey: n.PublicKey, AllowedIP: wg.HostIP(n.WGAddress)})
		newPubkeys[n.PublicKey] = struct{}{}
	}
	removePeers := staleClusterWGPeers(existing, latest.Status.Nodes, newPubkeys)
	if len(wgPeers) > 0 || len(removePeers) > 0 {
		jobName := fmt.Sprintf("fleet-cluster-wg-%s-%d", clusterName, time.Now().Unix())
		wgJob := jobs.WgSyncJobMulti(latest.Namespace, jobName, wgPeers, removePeers)
		_ = controllerutil.SetControllerReference(&latest, wgJob, r.Scheme)
		_ = r.Create(ctx, wgJob)
	}
	latest.Status.Nodes = make([]fleetv1alpha1.FleetClusterJoinNodeStatus, 0, len(nodeWG))
	for _, n := range nodeWG {
		latest.Status.Nodes = append(latest.Status.Nodes, fleetv1alpha1.FleetClusterJoinNodeStatus{
			Hostname:           n.Hostname,
			Address:            n.Address,
			WireguardAddress:   n.WGAddress,
			WireguardPublicKey: n.PublicKey,
		})
	}
	return r.Status().Update(ctx, &latest)
}

func (r *ClusterJoinReconciler) pollNodeOperationJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	op := fcj.Status.NodeOperation
	if op == nil || op.JobName == "" {
		return ctrl.Result{}, nil
	}
	done, ok, msg := r.jobDone(ctx, fcj.Namespace, op.JobName)
	if !done {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if !ok {
		return r.finishNodeOperation(ctx, fcj, clusterName, false, "Node operation failed: "+msg)
	}
	return r.finishNodeOperation(ctx, fcj, clusterName, true, "Node operation completed")
}

func (r *ClusterJoinReconciler) finishNodeOperation(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string, ok bool, message string) (ctrl.Result, error) {
	op := fcj.Status.NodeOperation
	if ok && op != nil {
		switch op.Type {
		case fleetv1alpha1.NodeOpTypeRemoveWorker:
			if err := r.removeNodeFromSpec(ctx, fcj, op.Node); err != nil {
				return r.failNodeOperation(ctx, fcj, err)
			}
			if err := r.syncNodeWGForSpec(ctx, fcj, clusterName); err != nil {
				return r.failNodeOperation(ctx, fcj, err)
			}
		}
		key := client.ObjectKeyFromObject(fcj)
		if err := r.Get(ctx, key, fcj); err != nil {
			return r.failNodeOperation(ctx, fcj, err)
		}
		if failMsg := r.postValidateNodeOperation(ctx, fcj, clusterName, op); failMsg != "" {
			return r.finishNodeOperation(ctx, fcj, clusterName, false, failMsg)
		}
	}
	key := client.ObjectKeyFromObject(fcj)
	ns := fcj.Namespace
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		if latest.Status.NodeOperation == nil {
			latest.Status.NodeOperation = &fleetv1alpha1.ClusterNodeOperationStatus{}
		}
		op := latest.Status.NodeOperation
		phase := fleetv1alpha1.NodeOpPhaseFailed
		if ok {
			phase = fleetv1alpha1.NodeOpPhaseSucceeded
		}
		if op.Phase != fleetv1alpha1.NodeOpPhaseSucceeded && op.Phase != fleetv1alpha1.NodeOpPhaseFailed {
			jobName := op.JobName
			logTail := r.captureNodeOpLogTail(ctx, ns, jobName)
			finishedAt := metav1.Now()
			rec := recordFromNodeOp(op, phase, message, logTail, finishedAt)
			latest.Status.NodeOperationHistory = prependNodeOpHistory(latest.Status.NodeOperationHistory, rec)
		}
		latest.Status.NodeOperation.JobName = ""
		latest.Status.NodeOperation.Message = message
		latest.Status.NodeOperation.Phase = phase
		latest.Status.Message = message
		return r.Status().Update(ctx, &latest)
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ClusterJoinReconciler) removeNodeFromSpec(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, hostname string) error {
	key := client.ObjectKeyFromObject(fcj)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if err := r.Get(ctx, key, &latest); err != nil {
			return err
		}
		out := make([]fleetv1alpha1.FleetClusterJoinNodeSpec, 0, len(latest.Spec.Nodes))
		for _, n := range latest.Spec.Nodes {
			hn := n.Hostname
			if hn == hostname {
				continue
			}
			out = append(out, n)
		}
		latest.Spec.Nodes = out
		return r.Update(ctx, &latest)
	})
}

func (r *ClusterJoinReconciler) postValidateNodeOperation(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string, op *fleetv1alpha1.ClusterNodeOperationStatus) string {
	req := nodeOperationValidateRequest(fcj, op)
	var nodeInfos []nodeops.NodeInfo
	list, err := spoke.ListNodes(ctx, spoke.ListParams{
		HubClient:             r.Client,
		ClusterName:           clusterName,
		EdgeClustersNamespace: r.Cfg.EdgeClustersCMNamespace,
		EdgeClustersName:      r.Cfg.EdgeClustersCMName,
	})
	if err == nil && list != nil {
		nodeInfos = nodeops.NodesFromK8s(list.Items)
	}
	result := nodeops.Validate(nodeops.PhasePost, fcj, req, nodeInfos, false)
	if result.OK {
		return ""
	}
	for _, c := range result.Checks {
		if c.Status == nodeops.CheckFail {
			if c.Message != "" {
				return "Post-validation failed: " + c.Message
			}
			return "Post-validation failed: " + c.ID
		}
	}
	return "Post-validation failed"
}

func nodeOperationValidateRequest(fcj *fleetv1alpha1.FleetClusterJoin, op *fleetv1alpha1.ClusterNodeOperationStatus) *nodeops.Request {
	req := &nodeops.Request{
		Type:  op.Type,
		Node:  op.Node,
		Force: op.Force,
	}
	switch op.Type {
	case fleetv1alpha1.NodeOpTypeAddWorker:
		if spec, ok := findNodeSpec(fcj, op.Node); ok {
			req.Address = spec.Address
			req.Hostname = spec.Hostname
		}
	case fleetv1alpha1.NodeOpTypeReplaceControlPlane:
		req.OldNode = op.Node
		for _, n := range fcj.Spec.Nodes {
			if n.EffectiveRole() != fleetv1alpha1.NodeRoleControl {
				continue
			}
			if n.Hostname != op.Node {
				req.NewNode = &nodeops.NewNode{Address: n.Address, Hostname: n.Hostname}
				break
			}
		}
	}
	return req
}

func (r *ClusterJoinReconciler) failNodeOperation(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, err error) (ctrl.Result, error) {
	key := client.ObjectKeyFromObject(fcj)
	msg := err.Error()
	_ = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if gerr := r.Get(ctx, key, &latest); gerr != nil {
			return gerr
		}
		if latest.Status.NodeOperation == nil {
			latest.Status.NodeOperation = &fleetv1alpha1.ClusterNodeOperationStatus{}
		}
		op := latest.Status.NodeOperation
		if op.Phase != fleetv1alpha1.NodeOpPhaseSucceeded && op.Phase != fleetv1alpha1.NodeOpPhaseFailed {
			logTail := r.captureNodeOpLogTail(ctx, latest.Namespace, op.JobName)
			finishedAt := metav1.Now()
			rec := recordFromNodeOp(op, fleetv1alpha1.NodeOpPhaseFailed, msg, logTail, finishedAt)
			latest.Status.NodeOperationHistory = prependNodeOpHistory(latest.Status.NodeOperationHistory, rec)
		}
		latest.Status.NodeOperation.Phase = fleetv1alpha1.NodeOpPhaseFailed
		latest.Status.NodeOperation.JobName = ""
		latest.Status.NodeOperation.Message = msg
		latest.Status.Message = msg
		return r.Status().Update(ctx, &latest)
	})
	return ctrl.Result{}, err
}

func (r *ClusterJoinReconciler) captureNodeOpLogTail(ctx context.Context, ns, jobName string) string {
	if strings.TrimSpace(jobName) == "" || r.Clientset == nil {
		return ""
	}
	logs, err := ansiblelogs.PodLogs(ctx, r.Clientset, ns, jobName)
	if err != nil {
		return ""
	}
	return ansiblelogs.Tail(logs, nodeops.MaxLogTailBytes)
}

func prependNodeOpHistory(history []fleetv1alpha1.ClusterNodeOperationRecord, rec fleetv1alpha1.ClusterNodeOperationRecord) []fleetv1alpha1.ClusterNodeOperationRecord {
	out := append([]fleetv1alpha1.ClusterNodeOperationRecord{rec}, history...)
	if len(out) > nodeops.MaxHistory {
		out = out[:nodeops.MaxHistory]
	}
	return out
}

func recordFromNodeOp(op *fleetv1alpha1.ClusterNodeOperationStatus, phase, message, logTail string, finishedAt metav1.Time) fleetv1alpha1.ClusterNodeOperationRecord {
	rec := fleetv1alpha1.ClusterNodeOperationRecord{
		ID:        op.ID,
		Type:      op.Type,
		Node:      op.Node,
		Phase:     phase,
		JobName:   op.JobName,
		Message:   message,
		Force:     op.Force,
		ActorKind: op.ActorKind,
		ActorName: op.ActorName,
		LogTail:   logTail,
		FinishedAt: &finishedAt,
	}
	if op.StartedAt != nil {
		t := *op.StartedAt
		rec.StartedAt = &t
	}
	return rec
}
