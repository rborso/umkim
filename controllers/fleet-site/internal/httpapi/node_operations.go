package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const annotationNodeOperationRequest = "fleet.umkim.com/node-operation-request"

var errClusterNodeOperationBusy = errors.New("cluster node operation already running")

type addWorkerBody struct {
	Address  string            `json:"address"`
	Hostname string            `json:"hostname,omitempty"`
	Role     string            `json:"role,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

type replaceControlPlaneBody struct {
	OldNode string         `json:"oldNode"`
	NewNode replaceNewNode `json:"newNode"`
	Force   bool           `json:"force,omitempty"`
}

type replaceNewNode struct {
	Address  string            `json:"address"`
	Hostname string            `json:"hostname,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

type nodeOperationRequest struct {
	Type     string            `json:"type"`
	Node     string            `json:"node,omitempty"`
	Address  string            `json:"address,omitempty"`
	Hostname string            `json:"hostname,omitempty"`
	Role     string            `json:"role,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	OldNode   string          `json:"oldNode,omitempty"`
	NewNode   *replaceNewNode `json:"newNode,omitempty"`
	Force     bool            `json:"force,omitempty"`
	ActorKind string          `json:"actorKind,omitempty"`
	ActorName string          `json:"actorName,omitempty"`
}

type nodeOperationResponse struct {
	Type              string `json:"type,omitempty"`
	Node              string `json:"node,omitempty"`
	Phase             string `json:"phase,omitempty"`
	Message           string `json:"message,omitempty"`
	Force             bool   `json:"force,omitempty"`
	ProvisioningPhase string `json:"provisioning_phase,omitempty"`
}

func (s *Server) handleReplaceControlPlane(w http.ResponseWriter, r *http.Request, clusterName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	actor, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req replaceControlPlaneBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	op := nodeOperationRequest{
		Type:      fleetv1alpha1.NodeOpTypeReplaceControlPlane,
		OldNode:   strings.TrimSpace(req.OldNode),
		ActorKind: actor.Kind,
		ActorName: actor.Name,
		NewNode: &replaceNewNode{
			Address:  strings.TrimSpace(req.NewNode.Address),
			Hostname: strings.TrimSpace(req.NewNode.Hostname),
			Labels:   req.NewNode.Labels,
		},
		Force: req.Force || queryParamBool(r, "force"),
	}
	s.submitNodeOperation(r.Context(), w, clusterName, op)
}

func (s *Server) handleAddWorker(w http.ResponseWriter, r *http.Request, clusterName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	actor, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req addWorkerBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = fleetv1alpha1.NodeRoleWorker
	}
	if role != fleetv1alpha1.NodeRoleWorker {
		http.Error(w, "only worker role is supported for add node", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Address) == "" {
		http.Error(w, "address is required", http.StatusBadRequest)
		return
	}
	op := nodeOperationRequest{
		Type:      fleetv1alpha1.NodeOpTypeAddWorker,
		Address:   strings.TrimSpace(req.Address),
		Hostname:  strings.TrimSpace(req.Hostname),
		Role:      role,
		Labels:    req.Labels,
		ActorKind: actor.Kind,
		ActorName: actor.Name,
	}
	s.submitNodeOperation(r.Context(), w, clusterName, op)
}

func (s *Server) handleRemoveWorker(w http.ResponseWriter, r *http.Request, clusterName, nodeName string) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	actor, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	op := nodeOperationRequest{
		Type:      fleetv1alpha1.NodeOpTypeRemoveWorker,
		Node:      strings.TrimSpace(nodeName),
		Force:     queryParamBool(r, "force"),
		ActorKind: actor.Kind,
		ActorName: actor.Name,
	}
	s.submitNodeOperation(r.Context(), w, clusterName, op)
}

func (s *Server) submitNodeOperation(ctx context.Context, w http.ResponseWriter, clusterName string, op nodeOperationRequest) {
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		http.Error(w, "cluster is not ready", http.StatusConflict)
		return
	}
	if op.Type == fleetv1alpha1.NodeOpTypeReplaceControlPlane && fcj.Spec.Topology == fleetv1alpha1.ClusterTopologySingleNode {
		http.Error(w, "replace control plane is not supported for single_node clusters", http.StatusBadRequest)
		return
	}
	if !s.preValidateNodeOperation(ctx, w, clusterName, nodeOperationRequestFromAPI(op, op.Force)) {
		return
	}
	if fcj.Status.NodeOperation != nil && fcj.Status.NodeOperation.Phase == fleetv1alpha1.NodeOpPhaseRunning {
		http.Error(w, errClusterNodeOperationBusy.Error(), http.StatusConflict)
		return
	}
	if fcj.Status.NodeOperation != nil && fcj.Status.NodeOperation.Phase == fleetv1alpha1.NodeOpPhasePending {
		http.Error(w, errClusterNodeOperationBusy.Error(), http.StatusConflict)
		return
	}
	if fcj.Status.Storage.Phase == fleetv1alpha1.StoragePhasePending ||
		fcj.Status.Storage.Phase == fleetv1alpha1.StoragePhaseApplying {
		http.Error(w, "cluster storage operation already running", http.StatusConflict)
		return
	}
	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if s.clusterReadOnly(clusterName, edge) {
		http.Error(w, "cluster is read-only", http.StatusForbidden)
		return
	}
	raw, err := json.Marshal(op)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	original := fcj.DeepCopy()
	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	fcj.Annotations[annotationNodeOperationRequest] = string(raw)
	if err := s.Client.Patch(ctx, fcj, client.MergeFrom(original)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	node := strings.TrimSpace(op.Node)
	if node == "" {
		node = strings.TrimSpace(op.Hostname)
	}
	if node == "" {
		node = strings.TrimSpace(op.OldNode)
	}
	writeJSON(w, http.StatusAccepted, nodeOperationResponse{
		Type:              op.Type,
		Node:              node,
		Phase:             fleetv1alpha1.NodeOpPhasePending,
		Message:           "Node operation accepted",
		Force:             op.Force,
		ProvisioningPhase: fcj.Status.Phase,
	})
}

func nodeOperationFromStatus(fcj *fleetv1alpha1.FleetClusterJoin) *nodeOperationResponse {
	if fcj.Status.NodeOperation == nil {
		return nil
	}
	op := fcj.Status.NodeOperation
	return &nodeOperationResponse{
		Type:              op.Type,
		Node:              op.Node,
		Phase:             op.Phase,
		Message:           op.Message,
		Force:             op.Force,
		ProvisioningPhase: fcj.Status.Phase,
	}
}
