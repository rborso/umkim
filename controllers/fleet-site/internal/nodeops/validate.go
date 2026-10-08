package nodeops

import (
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

type Phase string

const (
	PhasePre  Phase = "pre"
	PhasePost Phase = "post"
)

type CheckStatus string

const (
	CheckPass CheckStatus = "pass"
	CheckFail CheckStatus = "fail"
	CheckWarn CheckStatus = "warn"
)

type Check struct {
	ID      string      `json:"id"`
	Status  CheckStatus `json:"status"`
	Message string      `json:"message,omitempty"`
}

type Result struct {
	OK     bool    `json:"ok"`
	Phase  Phase   `json:"phase"`
	Type   string  `json:"type,omitempty"`
	Checks []Check `json:"checks"`
}

type NodeInfo struct {
	Name    string
	Address string
	Roles   []string
	Ready   bool
}

type NewNode struct {
	Address  string            `json:"address"`
	Hostname string            `json:"hostname,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

type Request struct {
	Type     string            `json:"type"`
	Node     string            `json:"node,omitempty"`
	Address  string            `json:"address,omitempty"`
	Hostname string            `json:"hostname,omitempty"`
	Role     string            `json:"role,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	OldNode  string            `json:"oldNode,omitempty"`
	NewNode  *NewNode          `json:"newNode,omitempty"`
	Force    bool              `json:"force,omitempty"`
}

func NodesFromK8s(items []corev1.Node) []NodeInfo {
	out := make([]NodeInfo, 0, len(items))
	for i := range items {
		n := &items[i]
		out = append(out, NodeInfo{
			Name:    n.Name,
			Address: nodeInternalIP(n),
			Roles:   nodeRoles(n),
			Ready:   nodeReady(n),
		})
	}
	return out
}

func nodeInternalIP(n *corev1.Node) string {
	for _, a := range n.Status.Addresses {
		if a.Type == corev1.NodeInternalIP && strings.TrimSpace(a.Address) != "" {
			return strings.TrimSpace(a.Address)
		}
	}
	return ""
}

func Validate(phase Phase, fcj *fleetv1alpha1.FleetClusterJoin, req *Request, nodes []NodeInfo, readOnly bool) Result {
	res := Result{
		Phase: phase,
		Checks: []Check{},
	}
	if req != nil {
		res.Type = req.Type
	}
	add := func(id string, status CheckStatus, msg string) {
		res.Checks = append(res.Checks, Check{ID: id, Status: status, Message: msg})
		if status == CheckFail {
			res.OK = false
		}
	}
	if phase == PhasePre {
		res.OK = true
		validatePre(fcj, req, nodes, readOnly, add)
	} else {
		res.OK = true
		validatePost(fcj, req, nodes, add)
	}
	return res
}

func validatePre(fcj *fleetv1alpha1.FleetClusterJoin, req *Request, nodes []NodeInfo, readOnly bool, add func(string, CheckStatus, string)) {
	if fcj == nil {
		add("cluster", CheckFail, "cluster not found")
		return
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		add("cluster_ready", CheckFail, fmt.Sprintf("cluster phase is %q, must be Ready", fcj.Status.Phase))
	} else {
		add("cluster_ready", CheckPass, "cluster is Ready")
	}
	if readOnly {
		add("cluster_writable", CheckFail, "cluster is read-only")
	} else {
		add("cluster_writable", CheckPass, "cluster accepts mutations")
	}
	if fcj.Status.NodeOperation != nil {
		switch fcj.Status.NodeOperation.Phase {
		case fleetv1alpha1.NodeOpPhasePending, fleetv1alpha1.NodeOpPhaseRunning:
			add("node_operation_idle", CheckFail, "another node operation is in progress")
		default:
			add("node_operation_idle", CheckPass, "no node operation in progress")
		}
	} else {
		add("node_operation_idle", CheckPass, "no node operation in progress")
	}
	if req == nil || req.Type == "" {
		add("operation_type", CheckFail, "operation type is required")
		return
	}
	if len(nodes) == 0 {
		add("spoke_api", CheckWarn, "could not list spoke nodes (post-checks may be limited)")
	} else {
		add("spoke_api", CheckPass, fmt.Sprintf("listed %d spoke node(s)", len(nodes)))
	}
	switch req.Type {
	case fleetv1alpha1.NodeOpTypeAddWorker:
		validatePreAddWorker(fcj, req, nodes, add)
	case fleetv1alpha1.NodeOpTypeRemoveWorker:
		validatePreRemoveWorker(fcj, req, nodes, add)
	case fleetv1alpha1.NodeOpTypeReplaceControlPlane:
		validatePreReplaceControl(fcj, req, nodes, add)
	default:
		add("operation_type", CheckFail, fmt.Sprintf("unsupported operation type %q", req.Type))
	}
}

func validatePreAddWorker(fcj *fleetv1alpha1.FleetClusterJoin, req *Request, nodes []NodeInfo, add func(string, CheckStatus, string)) {
	addr := strings.TrimSpace(req.Address)
	if addr == "" {
		add("address", CheckFail, "address is required")
	} else {
		add("address", CheckPass, "worker address provided")
	}
	for _, n := range fcj.Spec.Nodes {
		if strings.TrimSpace(n.Address) == addr && addr != "" {
			add("address_unique", CheckFail, fmt.Sprintf("address %s is already in cluster spec", addr))
			return
		}
	}
	if addr != "" {
		add("address_unique", CheckPass, "address is not already in cluster spec")
	}
	hn := strings.TrimSpace(req.Hostname)
	if hn != "" {
		if _, ok := findSpecNode(fcj, hn); ok {
			add("hostname_unique", CheckFail, fmt.Sprintf("hostname %s is already in cluster spec", hn))
		} else if nodeByName(nodes, hn) != nil {
			add("hostname_unique", CheckFail, fmt.Sprintf("hostname %s already exists in the cluster", hn))
		} else {
			add("hostname_unique", CheckPass, "hostname is available")
		}
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = fleetv1alpha1.NodeRoleWorker
	}
	if role != fleetv1alpha1.NodeRoleWorker {
		add("role", CheckFail, "only worker role is supported")
	} else {
		add("role", CheckPass, "worker role")
	}
}

func validatePreRemoveWorker(fcj *fleetv1alpha1.FleetClusterJoin, req *Request, nodes []NodeInfo, add func(string, CheckStatus, string)) {
	node := strings.TrimSpace(req.Node)
	if node == "" {
		add("node", CheckFail, "node name is required")
		return
	}
	add("node", CheckPass, "node name provided")
	spec, inSpec := findSpecNode(fcj, node)
	k8s := nodeByName(nodes, node)
	if !inSpec && k8s == nil {
		add("node_exists", CheckFail, fmt.Sprintf("node %s not found in cluster spec or Kubernetes", node))
	} else {
		add("node_exists", CheckPass, "node found")
	}
	isWorker := inSpec && spec.EffectiveRole() == fleetv1alpha1.NodeRoleWorker
	if k8s != nil {
		isWorker = isWorker || isWorkerNode(k8s)
	}
	if !isWorker {
		add("node_is_worker", CheckFail, fmt.Sprintf("node %s is not a worker", node))
	} else {
		add("node_is_worker", CheckPass, "node is a worker")
	}
	workers := countWorkers(fcj, nodes)
	if workers <= 1 && !req.Force {
		add("worker_count", CheckWarn, "removing the last worker node")
	} else {
		add("worker_count", CheckPass, fmt.Sprintf("%d worker node(s) in cluster", workers))
	}
	if k8s != nil && !k8s.Ready && !req.Force {
		add("node_reachable", CheckWarn, "node is not Ready; use force to remove offline nodes")
	} else if k8s == nil && !req.Force {
		add("node_reachable", CheckWarn, "node is not in Kubernetes; use force for offline removal")
	} else {
		add("node_reachable", CheckPass, "node reachability acceptable for removal")
	}
}

func validatePreReplaceControl(fcj *fleetv1alpha1.FleetClusterJoin, req *Request, nodes []NodeInfo, add func(string, CheckStatus, string)) {
	if fcj.Spec.Topology == fleetv1alpha1.ClusterTopologySingleNode {
		add("topology", CheckFail, "replace control plane is not supported for single_node clusters")
		return
	}
	oldNode := strings.TrimSpace(req.OldNode)
	if oldNode == "" {
		add("old_node", CheckFail, "oldNode is required")
		return
	}
	add("old_node", CheckPass, "oldNode provided")
	k8s := nodeByName(nodes, oldNode)
	if k8s != nil && !isControlNode(k8s) {
		add("old_node_is_control", CheckFail, fmt.Sprintf("node %s is not a control-plane node", oldNode))
	} else {
		add("old_node_is_control", CheckPass, "old node is control plane")
	}
	if req.NewNode == nil || strings.TrimSpace(req.NewNode.Address) == "" {
		add("new_address", CheckFail, "newNode.address is required")
	} else {
		add("new_address", CheckPass, "replacement address provided")
	}
	addr := ""
	newHN := ""
	if req.NewNode != nil {
		addr = strings.TrimSpace(req.NewNode.Address)
		newHN = strings.TrimSpace(req.NewNode.Hostname)
	}
	oldAddr := ""
	if k8s != nil {
		oldAddr = strings.TrimSpace(k8s.Address)
	}
	if oldAddr == "" {
		if spec, ok := findSpecNode(fcj, oldNode); ok {
			oldAddr = strings.TrimSpace(spec.Address)
		}
	}
	for _, n := range fcj.Spec.Nodes {
		nAddr := strings.TrimSpace(n.Address)
		if nAddr == "" || nAddr != addr {
			continue
		}
		if n.Hostname == oldNode || (oldAddr != "" && nAddr == oldAddr) {
			continue
		}
		// Idempotent retry: replacement hostname already owns the new address.
		if newHN != "" && n.Hostname == newHN {
			continue
		}
		add("new_address_unique", CheckFail, fmt.Sprintf("address %s is already used by %s", addr, n.Hostname))
		return
	}
	if addr != "" {
		add("new_address_unique", CheckPass, "replacement address is available")
	}
	if fcj.Spec.Topology == fleetv1alpha1.ClusterTopologyHA3Node {
		controls := countControlNodes(fcj, nodes)
		if controls < 3 {
			add("ha_control_count", CheckWarn, fmt.Sprintf("HA topology expects 3 control nodes; cluster reports %d", controls))
		} else {
			add("ha_control_count", CheckPass, "HA control-plane count looks valid")
		}
	}
	if k8s != nil && !k8s.Ready && !req.Force {
		add("old_node_reachable", CheckWarn, "old control-plane node is not Ready; use force when unreachable")
	} else {
		add("old_node_reachable", CheckPass, "old node reachability acceptable")
	}
}

func validatePost(fcj *fleetv1alpha1.FleetClusterJoin, req *Request, nodes []NodeInfo, add func(string, CheckStatus, string)) {
	if fcj == nil || req == nil || req.Type == "" {
		add("operation", CheckFail, "operation context missing")
		return
	}
	target := strings.TrimSpace(req.Node)
	switch req.Type {
	case fleetv1alpha1.NodeOpTypeAddWorker:
		if target == "" {
			target = strings.TrimSpace(req.Hostname)
		}
		if _, ok := findSpecNode(fcj, target); ok {
			add("spec_updated", CheckPass, fmt.Sprintf("worker %s is in cluster spec", target))
		} else {
			add("spec_updated", CheckFail, fmt.Sprintf("worker %s not found in cluster spec", target))
		}
		if len(nodes) == 0 {
			add("node_joined", CheckWarn, "could not verify node in Kubernetes")
			return
		}
		k8s := nodeByName(nodes, target)
		if k8s == nil {
			add("node_joined", CheckFail, fmt.Sprintf("node %s not found in Kubernetes", target))
		} else if !k8s.Ready {
			add("node_joined", CheckFail, fmt.Sprintf("node %s is not Ready", target))
		} else if !isWorkerNode(k8s) {
			add("node_joined", CheckWarn, fmt.Sprintf("node %s is not labeled as worker", target))
		} else {
			add("node_joined", CheckPass, fmt.Sprintf("node %s is Ready", target))
		}
	case fleetv1alpha1.NodeOpTypeRemoveWorker:
		if target == "" {
			target = strings.TrimSpace(req.Node)
		}
		if _, ok := findSpecNode(fcj, target); ok {
			add("spec_updated", CheckFail, fmt.Sprintf("node %s is still in cluster spec", target))
		} else {
			add("spec_updated", CheckPass, "node removed from cluster spec")
		}
		if len(nodes) == 0 {
			add("node_removed", CheckWarn, "could not verify node removal in Kubernetes")
			return
		}
		if nodeByName(nodes, target) != nil {
			add("node_removed", CheckFail, fmt.Sprintf("node %s still exists in Kubernetes", target))
		} else {
			add("node_removed", CheckPass, "node removed from Kubernetes")
		}
	case fleetv1alpha1.NodeOpTypeReplaceControlPlane:
		oldNode := strings.TrimSpace(req.OldNode)
		if oldNode == "" {
			oldNode = target
		}
		newHN := oldNode
		if req.NewNode != nil && strings.TrimSpace(req.NewNode.Hostname) != "" {
			newHN = strings.TrimSpace(req.NewNode.Hostname)
		}
		if spec, ok := findSpecNode(fcj, newHN); ok && strings.TrimSpace(spec.Address) != "" {
			add("spec_updated", CheckPass, fmt.Sprintf("replacement node %s is in cluster spec", newHN))
		} else {
			add("spec_updated", CheckFail, "replacement control node not found in cluster spec")
		}
		if len(nodes) == 0 {
			add("control_replaced", CheckWarn, "could not verify control-plane replacement in Kubernetes")
			return
		}
		newNode := nodeByName(nodes, newHN)
		if newNode == nil {
			add("control_replaced", CheckFail, fmt.Sprintf("replacement node %s not found in Kubernetes", newHN))
		} else if !newNode.Ready {
			add("control_replaced", CheckFail, fmt.Sprintf("replacement node %s is not Ready", newHN))
		} else if !isControlNode(newNode) {
			add("control_replaced", CheckWarn, fmt.Sprintf("replacement node %s is not labeled as control plane", newHN))
		} else {
			add("control_replaced", CheckPass, fmt.Sprintf("replacement node %s is Ready", newHN))
		}
		if oldNode != newHN && nodeByName(nodes, oldNode) != nil {
			st := CheckFail
			if req.Force {
				st = CheckWarn
			}
			add("old_node_removed", st, fmt.Sprintf("old node %s still appears in Kubernetes", oldNode))
		} else {
			add("old_node_removed", CheckPass, "old control-plane node is gone or renamed")
		}
	default:
		add("operation_type", CheckFail, fmt.Sprintf("unsupported operation type %q", req.Type))
	}
}

func findSpecNode(fcj *fleetv1alpha1.FleetClusterJoin, hostname string) (fleetv1alpha1.FleetClusterJoinNodeSpec, bool) {
	for _, n := range fcj.Spec.Nodes {
		if n.Hostname == hostname {
			return n, true
		}
	}
	return fleetv1alpha1.FleetClusterJoinNodeSpec{}, false
}

func nodeByName(nodes []NodeInfo, name string) *NodeInfo {
	for i := range nodes {
		if nodes[i].Name == name {
			return &nodes[i]
		}
	}
	return nil
}

func countWorkers(fcj *fleetv1alpha1.FleetClusterJoin, nodes []NodeInfo) int {
	seen := map[string]struct{}{}
	for _, n := range fcj.Spec.Nodes {
		if n.EffectiveRole() == fleetv1alpha1.NodeRoleWorker && n.Hostname != "" {
			seen[n.Hostname] = struct{}{}
		}
	}
	for _, n := range nodes {
		if isWorkerNode(&n) {
			seen[n.Name] = struct{}{}
		}
	}
	return len(seen)
}

func countControlNodes(fcj *fleetv1alpha1.FleetClusterJoin, nodes []NodeInfo) int {
	seen := map[string]struct{}{}
	for _, n := range fcj.Spec.Nodes {
		if n.EffectiveRole() == fleetv1alpha1.NodeRoleControl && n.Hostname != "" {
			seen[n.Hostname] = struct{}{}
		}
	}
	for _, n := range nodes {
		if isControlNode(&n) {
			seen[n.Name] = struct{}{}
		}
	}
	return len(seen)
}

func nodeRoles(n *corev1.Node) []string {
	if n == nil {
		return nil
	}
	out := []string{}
	for k := range n.Labels {
		if strings.HasPrefix(k, "node-role.kubernetes.io/") {
			role := strings.TrimPrefix(k, "node-role.kubernetes.io/")
			role = strings.TrimSuffix(role, "/")
			if role != "" {
				out = append(out, role)
			}
		}
	}
	return out
}

func nodeReady(n *corev1.Node) bool {
	if n == nil {
		return false
	}
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func isControlNode(n *NodeInfo) bool {
	for _, r := range n.Roles {
		if r == "control-plane" || r == "master" {
			return true
		}
	}
	return false
}

func isWorkerNode(n *NodeInfo) bool {
	for _, r := range n.Roles {
		if r == "worker" {
			return true
		}
	}
	return len(n.Roles) == 0
}
