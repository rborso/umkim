package clusters

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Cluster struct {
	Site              string   `yaml:"site"`
	DirectIngress     bool     `yaml:"direct_ingress"`
	ExposureMode      string   `yaml:"exposure_mode,omitempty"`
	TlsPolicy         string   `yaml:"tls_policy,omitempty"`
	IngressEndpointID string   `yaml:"ingress_endpoint_id,omitempty"`
	DomainBindings    []string `yaml:"domain_bindings,omitempty"`
	UpstreamHost      string   `yaml:"upstream_host"`
	EdgeWANIP         string   `yaml:"edge_wan_ip,omitempty"`
	TraefikHTTPPort   int      `yaml:"traefik_http_port"`
	TraefikHubHTTPPort int     `yaml:"traefik_hub_http_port,omitempty"`
	TraefikHTTPSPort  int      `yaml:"traefik_https_port,omitempty"`
}

const (
	exposureNone         = "none"
	exposurePortForward  = "port_forward"
	exposureReverseProxy = "reverse_proxy"

	TLSPolicyACMECluster = "acme_cluster"
)

// EffectiveExposureMode resolves cluster exposure with legacy direct_ingress fallback.
func (c Cluster) EffectiveExposureMode() string {
	if c.ExposureMode != "" && c.ExposureMode != "inherit" {
		return c.ExposureMode
	}
	if c.DirectIngress {
		return exposurePortForward
	}
	return exposureNone
}

// AllowsDirectSiteIngress is true when WAN can reach cluster Traefik (port-forward or reverse-proxy).
func (c Cluster) AllowsDirectSiteIngress() bool {
	mode := c.EffectiveExposureMode()
	return mode == exposurePortForward || mode == exposureReverseProxy
}

// SpokeACMEEnabled reports whether catalog spoke Ingresses should use cluster Let's Encrypt.
func (c Cluster) SpokeACMEEnabled(globalACME bool) bool {
	return globalACME && c.TlsPolicy == TLSPolicyACMECluster
}

type Registry struct {
	Clusters map[string]Cluster `yaml:"clusters"`
}

func Load(ctx context.Context, c client.Client, ns, name string) (Registry, error) {
	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		return Registry{}, fmt.Errorf("cluster configmap %s/%s: %w", ns, name, err)
	}
	raw, ok := cm.Data["clusters.yaml"]
	if !ok {
		return Registry{}, fmt.Errorf("cluster configmap missing clusters.yaml key")
	}
	var reg Registry
	if err := yaml.Unmarshal([]byte(raw), &reg); err != nil {
		return Registry{}, err
	}
	if reg.Clusters == nil {
		reg.Clusters = map[string]Cluster{}
	}
	return reg, nil
}

func (r Registry) Get(name string) (Cluster, bool) {
	c, ok := r.Clusters[name]
	return c, ok
}
