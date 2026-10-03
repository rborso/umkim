package httpapi

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

type nodeAddress struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

type nodeCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type nodeResourceMap struct {
	CPU              string `json:"cpu,omitempty"`
	Memory           string `json:"memory,omitempty"`
	EphemeralStorage string `json:"ephemeral_storage,omitempty"`
	Pods             string `json:"pods,omitempty"`
}

type nodeSummary struct {
	Name           string            `json:"name"`
	Roles          []string          `json:"roles,omitempty"`
	Ready          bool              `json:"ready"`
	Labels         map[string]string `json:"labels,omitempty"`
	Addresses      []nodeAddress     `json:"addresses,omitempty"`
	Capacity       *nodeResourceMap  `json:"capacity,omitempty"`
	Allocatable    *nodeResourceMap  `json:"allocatable,omitempty"`
	Usage          *nodeResourceMap  `json:"usage,omitempty"`
	Conditions     []nodeCondition   `json:"conditions,omitempty"`
	KubeletVersion string            `json:"kubelet_version,omitempty"`
	OSImage        string            `json:"os_image,omitempty"`
}

func nodeFromK8s(n *corev1.Node) nodeSummary {
	roles := []string{}
	filtered := map[string]string{}
	for k, v := range n.Labels {
		if strings.HasPrefix(k, "node-role.kubernetes.io/") {
			role := strings.TrimPrefix(k, "node-role.kubernetes.io/")
			if role == "" {
				role = "control-plane"
			}
			roles = append(roles, role)
		}
		if strings.HasPrefix(k, "fleet.") || strings.HasPrefix(k, "node-role.kubernetes.io/") {
			filtered[k] = v
		}
	}
	ready := false
	conds := make([]nodeCondition, 0, len(n.Status.Conditions))
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
			ready = true
		}
		conds = append(conds, nodeCondition{Type: string(c.Type), Status: string(c.Status), Reason: c.Reason, Message: c.Message})
	}
	addrs := make([]nodeAddress, 0, len(n.Status.Addresses))
	for _, a := range n.Status.Addresses {
		addrs = append(addrs, nodeAddress{Type: string(a.Type), Address: a.Address})
	}
	ns := nodeSummary{
		Name: n.Name, Roles: roles, Ready: ready, Labels: filtered,
		Addresses: addrs, Conditions: conds,
		Capacity:    resourceMapFrom(n.Status.Capacity),
		Allocatable: resourceMapFrom(n.Status.Allocatable),
	}
	if n.Status.NodeInfo.KubeletVersion != "" {
		ns.KubeletVersion = n.Status.NodeInfo.KubeletVersion
	}
	if n.Status.NodeInfo.OSImage != "" {
		ns.OSImage = n.Status.NodeInfo.OSImage
	}
	return ns
}

func resourceMapFrom(list corev1.ResourceList) *nodeResourceMap {
	if len(list) == 0 {
		return nil
	}
	m := &nodeResourceMap{}
	if q, ok := list[corev1.ResourceCPU]; ok {
		m.CPU = q.String()
	}
	if q, ok := list[corev1.ResourceMemory]; ok {
		m.Memory = q.String()
	}
	if q, ok := list[corev1.ResourceEphemeralStorage]; ok {
		m.EphemeralStorage = q.String()
	}
	if q, ok := list[corev1.ResourcePods]; ok {
		m.Pods = q.String()
	}
	return m
}

func attachNodeMetrics(ctx context.Context, cfg *rest.Config, nodes []nodeSummary) {
	if cfg == nil {
		return
	}
	mc, err := metricsclient.NewForConfig(cfg)
	if err != nil {
		return
	}
	list, err := mc.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	usageByName := map[string]corev1.ResourceList{}
	for _, nm := range list.Items {
		usageByName[nm.Name] = nm.Usage
	}
	for i := range nodes {
		if u, ok := usageByName[nodes[i].Name]; ok {
			nodes[i].Usage = resourceMapFrom(u)
		}
	}
}

func spokeMetricsConfig(base *rest.Config, clusterName string) *rest.Config {
	if base == nil {
		return nil
	}
	cfg := rest.CopyConfig(base)
	cfg.Host = strings.TrimSuffix(cfg.Host, "/") +
		"/apis/cluster.open-cluster-management.io/v1/managedclusters/" + clusterName + "/proxy"
	cfg.APIPath = ""
	cfg.GroupVersion = &metricsv1beta1.SchemeGroupVersion
	return cfg
}

func enrichHubNodes(ctx context.Context, cs kubernetes.Interface, cfg *rest.Config) []nodeSummary {
	list, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	out := make([]nodeSummary, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, nodeFromK8s(&list.Items[i]))
	}
	attachNodeMetrics(ctx, cfg, out)
	return out
}
