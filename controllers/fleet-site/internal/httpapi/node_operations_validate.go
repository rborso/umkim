package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/nodeops"
	"github.com/rborso/umkim/controllers/fleet-site/internal/spoke"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func (s *Server) handleValidateNodeOperation(w http.ResponseWriter, r *http.Request, clusterName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req, err := parseNodeOperationRequest(body, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	phase := nodeops.PhasePre
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("phase")), "post") {
		phase = nodeops.PhasePost
	}
	s.runNodeOperationValidation(r.Context(), w, clusterName, phase, req)
}

func (s *Server) runNodeOperationValidation(ctx context.Context, w http.ResponseWriter, clusterName string, phase nodeops.Phase, req *nodeops.Request) {
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	readOnly := s.clusterReadOnly(clusterName, edge)

	var nodeInfos []nodeops.NodeInfo
	list, listErr := spoke.ListNodes(ctx, spoke.ListParams{
		HubClient:             s.Client,
		HubREST:               s.RESTConfig,
		ClusterName:           clusterName,
		EdgeClustersNamespace: s.Cfg.EdgeClustersCMNamespace,
		EdgeClustersName:      s.Cfg.EdgeClustersCMName,
	})
	if listErr == nil && list != nil {
		nodeInfos = nodeops.NodesFromK8s(list.Items)
	}

	if phase == nodeops.PhasePost {
		req = enrichPostValidateRequest(fcj, req)
	}

	result := nodeops.Validate(phase, fcj, req, nodeInfos, readOnly)
	status := http.StatusOK
	if !result.OK {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, result)
}

func parseNodeOperationRequest(body []byte, r *http.Request) (*nodeops.Request, error) {
	var raw nodeOperationRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, err
		}
	}
	if raw.Type == "" {
		return nil, errNodeOperationTypeRequired
	}
	out := &nodeops.Request{
		Type:     raw.Type,
		Node:     strings.TrimSpace(raw.Node),
		Address:  strings.TrimSpace(raw.Address),
		Hostname: strings.TrimSpace(raw.Hostname),
		Role:     strings.TrimSpace(raw.Role),
		OldNode:  strings.TrimSpace(raw.OldNode),
		Force:    raw.Force || queryParamBool(r, "force"),
	}
	if raw.NewNode != nil {
		out.NewNode = &nodeops.NewNode{
			Address:  strings.TrimSpace(raw.NewNode.Address),
			Hostname: strings.TrimSpace(raw.NewNode.Hostname),
		}
	}
	return out, nil
}

func nodeOperationRequestFromAPI(op nodeOperationRequest, force bool) *nodeops.Request {
	req := &nodeops.Request{
		Type:     op.Type,
		Node:     strings.TrimSpace(op.Node),
		Address:  strings.TrimSpace(op.Address),
		Hostname: strings.TrimSpace(op.Hostname),
		Role:     strings.TrimSpace(op.Role),
		OldNode:  strings.TrimSpace(op.OldNode),
		Force:    op.Force || force,
	}
	if op.NewNode != nil {
		req.NewNode = &nodeops.NewNode{
			Address:  strings.TrimSpace(op.NewNode.Address),
			Hostname: strings.TrimSpace(op.NewNode.Hostname),
		}
	}
	return req
}

func enrichPostValidateRequest(fcj *fleetv1alpha1.FleetClusterJoin, req *nodeops.Request) *nodeops.Request {
	if req == nil {
		req = &nodeops.Request{}
	}
	if fcj.Status.NodeOperation != nil {
		if req.Type == "" {
			req.Type = fcj.Status.NodeOperation.Type
		}
		if req.Node == "" {
			req.Node = fcj.Status.NodeOperation.Node
		}
		if req.Force != fcj.Status.NodeOperation.Force {
			req.Force = fcj.Status.NodeOperation.Force
		}
	}
	switch req.Type {
	case fleetv1alpha1.NodeOpTypeAddWorker:
		target := req.Node
		if target == "" {
			target = req.Hostname
		}
		if spec, ok := findNodeSpecInFCJ(fcj, target); ok {
			if req.Address == "" {
				req.Address = spec.Address
			}
			if req.Hostname == "" {
				req.Hostname = spec.Hostname
			}
		}
	case fleetv1alpha1.NodeOpTypeReplaceControlPlane:
		if req.OldNode == "" {
			req.OldNode = req.Node
		}
		if req.NewNode == nil {
			for _, n := range fcj.Spec.Nodes {
				if n.EffectiveRole() != fleetv1alpha1.NodeRoleControl {
					continue
				}
				if n.Hostname != req.OldNode {
					req.NewNode = &nodeops.NewNode{Address: n.Address, Hostname: n.Hostname}
					break
				}
			}
		}
	}
	return req
}

func findNodeSpecInFCJ(fcj *fleetv1alpha1.FleetClusterJoin, hostname string) (fleetv1alpha1.FleetClusterJoinNodeSpec, bool) {
	for _, n := range fcj.Spec.Nodes {
		if n.Hostname == hostname {
			return n, true
		}
	}
	return fleetv1alpha1.FleetClusterJoinNodeSpec{}, false
}

func (s *Server) preValidateNodeOperation(ctx context.Context, w http.ResponseWriter, clusterName string, req *nodeops.Request) bool {
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster not found", http.StatusNotFound)
			return false
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}
	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	readOnly := s.clusterReadOnly(clusterName, edge)

	var nodeInfos []nodeops.NodeInfo
	list, listErr := spoke.ListNodes(ctx, spoke.ListParams{
		HubClient:             s.Client,
		HubREST:               s.RESTConfig,
		ClusterName:           clusterName,
		EdgeClustersNamespace: s.Cfg.EdgeClustersCMNamespace,
		EdgeClustersName:      s.Cfg.EdgeClustersCMName,
	})
	if listErr == nil && list != nil {
		nodeInfos = nodeops.NodesFromK8s(list.Items)
	}
	result := nodeops.Validate(nodeops.PhasePre, fcj, req, nodeInfos, readOnly)
	if !result.OK {
		writeJSON(w, http.StatusUnprocessableEntity, result)
		return false
	}
	return true
}

var errNodeOperationTypeRequired = fmt.Errorf("operation type is required")
