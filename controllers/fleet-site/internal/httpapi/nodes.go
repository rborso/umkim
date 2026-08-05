package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type nodePatchBody struct {
	Labels         map[string]string `json:"labels,omitempty"`
	LabelsToRemove []string          `json:"labelsToRemove,omitempty"`
}

func (s *Server) handleClusterNodes(w http.ResponseWriter, r *http.Request, clusterName, sub string) {
	sub = strings.TrimPrefix(sub, "nodes")
	sub = strings.Trim(sub, "/")
	if sub == "" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleListNodes(r.Context(), w, clusterName)
		return
	}
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.handlePatchNode(r.Context(), w, r, clusterName, sub)
}

func (s *Server) spokeClientset(clusterName string) (kubernetes.Interface, error) {
	if s.RESTConfig == nil {
		return nil, fmt.Errorf("REST config not configured")
	}
	cfg := rest.CopyConfig(s.RESTConfig)
	cfg.Host = strings.TrimSuffix(cfg.Host, "/") +
		"/apis/cluster.open-cluster-management.io/v1/managedclusters/" + clusterName + "/proxy"
	cfg.APIPath = ""
	cfg.GroupVersion = nil
	return kubernetes.NewForConfig(cfg)
}

func (s *Server) localClientset() (kubernetes.Interface, error) {
	if s.RESTConfig == nil {
		return nil, fmt.Errorf("REST config not configured")
	}
	return kubernetes.NewForConfig(s.RESTConfig)
}

func (s *Server) handleListNodes(ctx context.Context, w http.ResponseWriter, clusterName string) {
	list, metricsCfg, err := s.listSpokeNodes(ctx, clusterName)
	if err != nil {
		http.Error(w, "cluster proxy failed: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	out := make([]nodeSummary, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, nodeFromK8s(&list.Items[i]))
	}
	attachNodeMetrics(ctx, metricsCfg, out)
	writeJSON(w, http.StatusOK, map[string]interface{}{"nodes": out})
}

func (s *Server) handlePatchNode(ctx context.Context, w http.ResponseWriter, r *http.Request, clusterName, nodeName string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var patch nodePatchBody
	if err := json.Unmarshal(body, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for k := range patch.Labels {
		if !allowedNodeLabel(k) {
			http.Error(w, "label not allowed: "+k, http.StatusBadRequest)
			return
		}
	}
	for _, k := range patch.LabelsToRemove {
		if !allowedNodeLabel(k) {
			http.Error(w, "label not allowed: "+k, http.StatusBadRequest)
			return
		}
	}

	cs, err := s.spokeClientset(clusterName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	node, err := cs.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		cs, err = s.directSpokeClientset(ctx, clusterName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		node, err = cs.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			http.Error(w, "node not found: "+err.Error(), http.StatusNotFound)
			return
		}
	}
	if node.Labels == nil {
		node.Labels = map[string]string{}
	}
	for k, v := range patch.Labels {
		node.Labels[k] = v
	}
	for _, k := range patch.LabelsToRemove {
		delete(node.Labels, k)
	}
	updated, err := cs.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		http.Error(w, "cluster proxy update failed: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, nodeFromK8s(updated))
}

func allowedNodeLabel(k string) bool {
	return strings.HasPrefix(k, "fleet.") || strings.HasPrefix(k, "node-role.kubernetes.io/")
}
