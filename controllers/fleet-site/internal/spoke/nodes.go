package spoke

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
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const kubeconfigSecretNS = "fleet-system"

var managedClusterGVK = schema.GroupVersionKind{
	Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
}

type ListParams struct {
	HubClient             client.Client
	HubREST               *rest.Config
	ClusterName           string
	EdgeClustersNamespace string
	EdgeClustersName      string
}

func ListNodes(ctx context.Context, p ListParams) (*corev1.NodeList, error) {
	if p.HubREST != nil {
		if cs, err := ocmProxyClientset(p.HubREST, p.ClusterName); err == nil {
			list, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			if err == nil {
				return list, nil
			}
		}
	}
	cfg, err := directRestConfig(ctx, p)
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	list, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list spoke nodes: %w", err)
	}
	return list, nil
}

func ocmProxyClientset(hubREST *rest.Config, clusterName string) (kubernetes.Interface, error) {
	cfg := rest.CopyConfig(hubREST)
	cfg.Host = strings.TrimSuffix(cfg.Host, "/") +
		"/apis/cluster.open-cluster-management.io/v1/managedclusters/" + clusterName + "/proxy"
	cfg.APIPath = ""
	cfg.GroupVersion = nil
	return kubernetes.NewForConfig(cfg)
}

func directRestConfig(ctx context.Context, p ListParams) (*rest.Config, error) {
	host := spokeAPIHost(ctx, p)
	if host == "" {
		return nil, fmt.Errorf("no spoke API host for cluster %s", p.ClusterName)
	}
	raw, err := spokeKubeconfigBytes(ctx, p.HubClient, p.ClusterName)
	if err != nil {
		return nil, err
	}
	kc := patchKubeconfigServer(string(raw), host)
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kc))
	if err != nil {
		return nil, fmt.Errorf("parse spoke kubeconfig: %w", err)
	}
	cfg.TLSClientConfig.Insecure = true
	cfg.TLSClientConfig.CAData = nil
	return cfg, nil
}

func spokeAPIHost(ctx context.Context, p ListParams) string {
	edge, _ := loadEdgeClusters(ctx, p.HubClient, p.EdgeClustersNamespace, p.EdgeClustersName)
	if e, ok := edge.Clusters[p.ClusterName]; ok && strings.TrimSpace(e.UpstreamHost) != "" {
		return strings.TrimSpace(e.UpstreamHost)
	}
	mc, err := getManagedCluster(ctx, p.HubClient, p.ClusterName)
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

func spokeKubeconfigBytes(ctx context.Context, c client.Client, clusterName string) ([]byte, error) {
	var secret corev1.Secret
	key := client.ObjectKey{Namespace: kubeconfigSecretNS, Name: "fleet-kubeconfig-" + clusterName}
	if err := c.Get(ctx, key, &secret); err != nil {
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

func patchKubeconfigServer(kc, host string) string {
	for _, old := range []string{
		"https://127.0.0.1:6443",
		"https://0.0.0.0:6443",
		"https://localhost:6443",
	} {
		kc = strings.ReplaceAll(kc, old, "https://"+host+":6443")
	}
	return kc
}

type edgeClustersFile struct {
	Clusters map[string]edgeClusterEntry `yaml:"clusters"`
}

type edgeClusterEntry struct {
	UpstreamHost string `yaml:"upstream_host"`
}

func loadEdgeClusters(ctx context.Context, c client.Client, ns, name string) (edgeClustersFile, error) {
	var cm corev1.ConfigMap
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return edgeClustersFile{Clusters: map[string]edgeClusterEntry{}}, nil
		}
		return edgeClustersFile{}, err
	}
	raw := cm.Data["clusters.yaml"]
	if raw == "" {
		return edgeClustersFile{Clusters: map[string]edgeClusterEntry{}}, nil
	}
	var f edgeClustersFile
	if err := yaml.Unmarshal([]byte(raw), &f); err != nil {
		return edgeClustersFile{}, err
	}
	if f.Clusters == nil {
		f.Clusters = map[string]edgeClusterEntry{}
	}
	return f, nil
}

func getManagedCluster(ctx context.Context, c client.Client, name string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(managedClusterGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: name}, obj); err != nil {
		return nil, err
	}
	return obj, nil
}
