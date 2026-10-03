package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var (
	gvrVirtualMachine = schema.GroupVersionResource{
		Group: "kubevirt.io", Version: "v1", Resource: "virtualmachines",
	}
	gvrVirtualMachineInstance = schema.GroupVersionResource{
		Group: "kubevirt.io", Version: "v1", Resource: "virtualmachineinstances",
	}
)

type vmSummary struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	Runnable  bool   `json:"runnable,omitempty"`
	Running   bool   `json:"running"`
	Printable string `json:"printableStatus,omitempty"`
	Phase     string `json:"phase,omitempty"`
	NodeName  string `json:"nodeName,omitempty"`
	IP        string `json:"ip,omitempty"`
	Age       string `json:"creationTimestamp,omitempty"`
}

type vmPowerBody struct {
	Action string `json:"action"`
	Force  bool   `json:"force,omitempty"`
}

func (s *Server) handleClusterVMs(w http.ResponseWriter, r *http.Request, clusterName, sub string) {
	sub = strings.TrimPrefix(sub, "vms")
	sub = strings.Trim(sub, "/")
	if sub == "" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleListVMs(r.Context(), w, clusterName)
		return
	}

	parts := strings.Split(sub, "/")
	if len(parts) < 2 {
		http.Error(w, "use /vms/{namespace}/{name}[/{action}]", http.StatusBadRequest)
		return
	}
	ns, name := parts[0], parts[1]
	restPath := ""
	if len(parts) > 2 {
		restPath = strings.Join(parts[2:], "/")
	}

	switch {
	case restPath == "" && r.Method == http.MethodGet:
		s.handleGetVM(r.Context(), w, clusterName, ns, name)
	case restPath == "power" && r.Method == http.MethodPost:
		s.handleVMPower(r.Context(), w, r, clusterName, ns, name)
	case (restPath == "console" || restPath == "serial") && (r.Method == http.MethodGet || r.Header.Get("Upgrade") != ""):
		s.handleVMConsoleWS(w, r, clusterName, ns, name, "console")
	case restPath == "vnc" && (r.Method == http.MethodGet || r.Header.Get("Upgrade") != ""):
		s.handleVMConsoleWS(w, r, clusterName, ns, name, "vnc")
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) spokeDynamic(ctx context.Context, clusterName string) (dynamic.Interface, *rest.Config, error) {
	// Prefer direct spoke kubeconfig: OCM cluster proxy often lacks CRD/subresource
	// coverage for kubevirt.io / subresources.kubevirt.io.
	if cfg, err := s.directSpokeRestConfig(ctx, clusterName); err == nil {
		if dyn, err := dynamic.NewForConfig(cfg); err == nil {
			_, probeErr := dyn.Resource(gvrVirtualMachine).Namespace("").List(ctx, metav1.ListOptions{Limit: 1})
			if probeErr == nil || isMissingKubeVirt(probeErr) {
				return dyn, cfg, nil
			}
		}
	}
	if s.RESTConfig == nil {
		return nil, nil, fmt.Errorf("no spoke access for cluster %s", clusterName)
	}
	cfg := rest.CopyConfig(s.RESTConfig)
	cfg.Host = strings.TrimSuffix(cfg.Host, "/") +
		"/apis/cluster.open-cluster-management.io/v1/managedclusters/" + clusterName + "/proxy"
	cfg.APIPath = ""
	cfg.GroupVersion = nil
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	return dyn, cfg, nil
}

func isProxyUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no route") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "dial tcp") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "service unavailable")
}

func isMissingKubeVirt(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return apierrors.IsNotFound(err) ||
		strings.Contains(msg, "no matches for kind") ||
		strings.Contains(msg, "could not find") ||
		strings.Contains(msg, "the server could not find the requested resource")
}

func (s *Server) handleListVMs(ctx context.Context, w http.ResponseWriter, clusterName string) {
	dyn, _, err := s.spokeDynamic(ctx, clusterName)
	if err != nil {
		http.Error(w, "spoke client failed: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	list, err := dyn.Resource(gvrVirtualMachine).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if isMissingKubeVirt(err) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"vms": []vmSummary{}, "message": "KubeVirt CRDs not installed"})
			return
		}
		http.Error(w, "list virtualmachines: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	vmiByKey := map[string]*unstructured.Unstructured{}
	if vmiList, err := dyn.Resource(gvrVirtualMachineInstance).Namespace("").List(ctx, metav1.ListOptions{}); err == nil {
		for i := range vmiList.Items {
			item := &vmiList.Items[i]
			vmiByKey[item.GetNamespace()+"/"+item.GetName()] = item
		}
	}
	out := make([]vmSummary, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, vmSummaryFrom(&list.Items[i], vmiByKey[list.Items[i].GetNamespace()+"/"+list.Items[i].GetName()]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"vms": out})
}

func (s *Server) handleGetVM(ctx context.Context, w http.ResponseWriter, clusterName, ns, name string) {
	dyn, _, err := s.spokeDynamic(ctx, clusterName)
	if err != nil {
		http.Error(w, "spoke client failed: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	vm, err := dyn.Resource(gvrVirtualMachine).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "virtualmachine not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	var vmi *unstructured.Unstructured
	if obj, err := dyn.Resource(gvrVirtualMachineInstance).Namespace(ns).Get(ctx, name, metav1.GetOptions{}); err == nil {
		vmi = obj
	}
	writeJSON(w, http.StatusOK, vmSummaryFrom(vm, vmi))
}

func (s *Server) handleVMPower(ctx context.Context, w http.ResponseWriter, r *http.Request, clusterName, ns, name string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req vmPowerBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "start", "on", "poweron", "power-on":
		action = "start"
	case "stop", "off", "poweroff", "power-off", "shutdown":
		action = "stop"
	case "restart", "reboot":
		action = "restart"
	default:
		http.Error(w, "action must be start, stop, or restart", http.StatusBadRequest)
		return
	}

	_, cfg, err := s.spokeDynamic(ctx, clusterName)
	if err != nil {
		http.Error(w, "spoke client failed: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	if err := s.vmSubresource(ctx, cfg, ns, name, action, req.Force); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"namespace": ns,
		"name":      name,
		"action":    action,
		"force":     req.Force,
		"status":    "accepted",
	})
}

func vmSummaryFrom(vm, vmi *unstructured.Unstructured) vmSummary {
	sum := vmSummary{
		Namespace: vm.GetNamespace(),
		Name:      vm.GetName(),
		Age:       vm.GetCreationTimestamp().UTC().Format(metav1.RFC3339Micro),
	}
	if ps, ok, _ := unstructured.NestedString(vm.Object, "status", "printableStatus"); ok {
		sum.Printable = ps
	}
	if ready, ok, _ := unstructured.NestedBool(vm.Object, "status", "ready"); ok {
		sum.Ready = ready
	}
	if run, ok, _ := unstructured.NestedBool(vm.Object, "spec", "running"); ok {
		sum.Runnable = run
	}
	if rs, ok, _ := unstructured.NestedString(vm.Object, "spec", "runStrategy"); ok && rs != "" {
		sum.Runnable = rs == "Always" || rs == "RerunOnFailure" || rs == "Manual"
	}
	if vmi != nil {
		if phase, ok, _ := unstructured.NestedString(vmi.Object, "status", "phase"); ok {
			sum.Phase = phase
			sum.Running = phase == "Running"
		}
		if node, ok, _ := unstructured.NestedString(vmi.Object, "status", "nodeName"); ok {
			sum.NodeName = node
		}
		if ifaces, ok, _ := unstructured.NestedSlice(vmi.Object, "status", "interfaces"); ok {
			for _, raw := range ifaces {
				m, _ := raw.(map[string]interface{})
				if m == nil {
					continue
				}
				if ip, _ := m["ipAddress"].(string); ip != "" {
					sum.IP = ip
					break
				}
			}
		}
	} else if sum.Printable == "Running" {
		sum.Running = true
		sum.Phase = "Running"
	}
	return sum
}
