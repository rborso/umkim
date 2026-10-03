package resourcemetrics

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const spokeKubeconfigSecretNS = "fleet-system"

var managedClusterGVK = schema.GroupVersionKind{
	Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
}

// Scraper collects node utilization from spoke clusters via OCM proxy or direct kubeconfig.
type Scraper struct {
	HubClient               client.Client
	HubREST                 *rest.Config
	EdgeClustersCMNamespace string
	EdgeClustersCMName      string
}

func (s *Scraper) ScrapeCluster(ctx context.Context, clusterName string) (Utilization, error) {
	nodes, cs, metricsCfg, err := s.listSpokeNodes(ctx, clusterName)
	if err != nil {
		return Utilization{}, err
	}
	usageByName, _ := s.listNodeUsage(ctx, metricsCfg)
	requestedByName, unscheduledReq, _ := s.listNodeRequested(ctx, cs)
	return AggregateCluster(nodes.Items, usageByName, requestedByName, unscheduledReq), nil
}

func (s *Scraper) listSpokeNodes(ctx context.Context, clusterName string) (*corev1.NodeList, kubernetes.Interface, *rest.Config, error) {
	if cs, err := s.spokeClientset(clusterName); err == nil {
		list, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err == nil {
			return list, cs, spokeMetricsConfig(s.HubREST, clusterName), nil
		}
	}
	cfg, err := s.directSpokeRestConfig(ctx, clusterName)
	if err != nil {
		return nil, nil, nil, err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	list, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("direct spoke API failed: %w", err)
	}
	return list, cs, cfg, nil
}

func (s *Scraper) listNodeRequested(ctx context.Context, cs kubernetes.Interface) (map[string]corev1.ResourceList, corev1.ResourceList, error) {
	out := map[string]corev1.ResourceList{}
	var unscheduled corev1.ResourceList
	if cs == nil {
		return out, nil, fmt.Errorf("no kubernetes client")
	}
	pods, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return out, nil, err
	}
	unscheduled = corev1.ResourceList{}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !podSchedulable(pod) {
			continue
		}
		req := podRequests(pod)
		if len(req) == 0 {
			continue
		}
		nodeName := pod.Spec.NodeName
		if nodeName == "" {
			for name, qty := range req {
				if cur, ok := unscheduled[name]; ok {
					cur.Add(qty)
					unscheduled[name] = cur
				} else {
					unscheduled[name] = qty.DeepCopy()
				}
			}
			continue
		}
		existing := out[nodeName]
		if existing == nil {
			existing = corev1.ResourceList{}
		}
		for name, qty := range req {
			if cur, ok := existing[name]; ok {
				cur.Add(qty)
				existing[name] = cur
			} else {
				existing[name] = qty.DeepCopy()
			}
		}
		out[nodeName] = existing
	}
	return out, unscheduled, nil
}

func (s *Scraper) listNodeUsage(ctx context.Context, cfg *rest.Config) (map[string]corev1.ResourceList, error) {
	out := map[string]corev1.ResourceList{}
	if cfg == nil {
		return out, fmt.Errorf("no metrics config")
	}
	mc, err := metricsclient.NewForConfig(cfg)
	if err != nil {
		return out, err
	}
	list, err := mc.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return out, err
	}
	for _, nm := range list.Items {
		out[nm.Name] = nm.Usage
	}
	return out, nil
}

func (s *Scraper) spokeClientset(clusterName string) (kubernetes.Interface, error) {
	if s.HubREST == nil {
		return nil, fmt.Errorf("hub REST config not configured")
	}
	cfg := rest.CopyConfig(s.HubREST)
	cfg.Host = strings.TrimSuffix(cfg.Host, "/") +
		"/apis/cluster.open-cluster-management.io/v1/managedclusters/" + clusterName + "/proxy"
	cfg.APIPath = ""
	cfg.GroupVersion = nil
	return kubernetes.NewForConfig(cfg)
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

func (s *Scraper) directSpokeRestConfig(ctx context.Context, clusterName string) (*rest.Config, error) {
	host := s.spokeAPIHost(ctx, clusterName)
	if host == "" {
		return nil, fmt.Errorf("no spoke API host for cluster %s", clusterName)
	}
	raw, err := s.spokeKubeconfigBytes(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	kc := patchSpokeKubeconfigServer(string(raw), host)
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kc))
	if err != nil {
		return nil, fmt.Errorf("parse spoke kubeconfig: %w", err)
	}
	cfg.TLSClientConfig.Insecure = true
	cfg.TLSClientConfig.CAData = nil
	return cfg, nil
}

func (s *Scraper) spokeAPIHost(ctx context.Context, clusterName string) string {
	if s.HubClient != nil && s.EdgeClustersCMNamespace != "" && s.EdgeClustersCMName != "" {
		edge, err := loadEdgeClusters(ctx, s.HubClient, s.EdgeClustersCMNamespace, s.EdgeClustersCMName)
		if err == nil {
			if e, ok := edge.Clusters[clusterName]; ok && strings.TrimSpace(e.UpstreamHost) != "" {
				return strings.TrimSpace(e.UpstreamHost)
			}
		}
	}
	mc, err := s.getManagedCluster(ctx, clusterName)
	if err != nil {
		return ""
	}
	labels := mc.GetLabels()
	if labels == nil {
		return ""
	}
	if v := strings.TrimSpace(labels["fleet.topology/wireguard_ip"]); v != "" {
		return v
	}
	return ""
}

func (s *Scraper) spokeKubeconfigBytes(ctx context.Context, clusterName string) ([]byte, error) {
	var secret corev1.Secret
	key := client.ObjectKey{Namespace: spokeKubeconfigSecretNS, Name: "fleet-kubeconfig-" + clusterName}
	if err := s.HubClient.Get(ctx, key, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("kubeconfig secret %s/%s not found", key.Namespace, key.Name)
		}
		return nil, err
	}
	raw := secret.Data["kubeconfig"]
	if len(raw) == 0 {
		raw = secret.Data["value"]
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("kubeconfig secret %s/%s is empty", key.Namespace, key.Name)
	}
	return raw, nil
}

func patchSpokeKubeconfigServer(kc, host string) string {
	for _, old := range []string{
		"https://127.0.0.1:6443",
		"https://0.0.0.0:6443",
		"https://localhost:6443",
	} {
		kc = strings.ReplaceAll(kc, old, "https://"+host+":6443")
	}
	return kc
}

func (s *Scraper) getManagedCluster(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(managedClusterGVK)
	if err := s.HubClient.Get(ctx, client.ObjectKey{Name: name}, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// ListAvailableManagedClusters returns joined hub-accepted clusters (excluding local-cluster).
func ListAvailableManagedClusters(ctx context.Context, c client.Client) ([]string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group: managedClusterGVK.Group, Version: managedClusterGVK.Version, Kind: managedClusterGVK.Kind + "List",
	})
	if err := c.List(ctx, list); err != nil {
		return nil, err
	}
	var names []string
	for _, item := range list.Items {
		name := item.GetName()
		if name == "" || name == "local-cluster" {
			continue
		}
		if managedClusterEligibleForMetrics(&item) {
			names = append(names, name)
		}
	}
	return names, nil
}

func managedClusterEligibleForMetrics(mc *unstructured.Unstructured) bool {
	accepted, found, _ := unstructured.NestedBool(mc.Object, "spec", "hubAcceptsClient")
	if !found || !accepted {
		return false
	}
	return managedClusterJoined(mc) || managedClusterAvailable(mc)
}

func managedClusterJoined(mc *unstructured.Unstructured) bool {
	conds, found, _ := unstructured.NestedSlice(mc.Object, "status", "conditions")
	if !found {
		return false
	}
	for _, c := range conds {
		m, _ := c.(map[string]interface{})
		if m == nil {
			continue
		}
		if m["type"] == "ManagedClusterJoined" && m["status"] == "True" {
			return true
		}
	}
	return false
}

func managedClusterAvailable(mc *unstructured.Unstructured) bool {
	conds, found, _ := unstructured.NestedSlice(mc.Object, "status", "conditions")
	if !found {
		return false
	}
	for _, c := range conds {
		m, _ := c.(map[string]interface{})
		if m == nil {
			continue
		}
		if m["type"] == "ManagedClusterConditionAvailable" && m["status"] == "True" {
			return true
		}
	}
	return false
}
