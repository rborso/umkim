package httpapi

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const spokeKubeconfigSecretNS = "fleet-system"

func (s *Server) listSpokeNodes(ctx context.Context, clusterName string) (*corev1.NodeList, *rest.Config, error) {
	if cs, err := s.spokeClientset(clusterName); err == nil {
		list, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err == nil {
			return list, spokeMetricsConfig(s.RESTConfig, clusterName), nil
		}
	}
	cfg, err := s.directSpokeRestConfig(ctx, clusterName)
	if err != nil {
		return nil, nil, err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	list, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("direct spoke API failed: %w", err)
	}
	return list, cfg, nil
}

func (s *Server) directSpokeClientset(ctx context.Context, clusterName string) (kubernetes.Interface, error) {
	cfg, err := s.directSpokeRestConfig(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

func (s *Server) directSpokeRestConfig(ctx context.Context, clusterName string) (*rest.Config, error) {
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
	// Spoke certs typically cover cluster-service / node IPs, not overlay WG addresses.
	cfg.TLSClientConfig.Insecure = true
	cfg.TLSClientConfig.CAData = nil
	return cfg, nil
}

func (s *Server) spokeAPIHost(ctx context.Context, clusterName string) string {
	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if e, ok := edge.Clusters[clusterName]; ok && strings.TrimSpace(e.UpstreamHost) != "" {
		return strings.TrimSpace(e.UpstreamHost)
	}
	mc, err := getManagedCluster(ctx, s.Client, clusterName)
	if err != nil {
		return ""
	}
	labels := mcLabels(mc)
	if v := strings.TrimSpace(labels["fleet.topology/wireguard_ip"]); v != "" {
		return v
	}
	return ""
}

func (s *Server) spokeKubeconfigBytes(ctx context.Context, clusterName string) ([]byte, error) {
	var secret corev1.Secret
	key := client.ObjectKey{Namespace: spokeKubeconfigSecretNS, Name: "fleet-kubeconfig-" + clusterName}
	if err := s.Client.Get(ctx, key, &secret); err != nil {
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
