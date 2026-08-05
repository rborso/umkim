package httpapi

import (
	"context"

	"github.com/rborso/umkim/controllers/fleet-site/internal/registry"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type edgeClusterEntry struct {
	Site              string                 `yaml:"site"`
	Management        bool                   `yaml:"management,omitempty"`
	ReadOnly          bool                   `yaml:"read_only,omitempty"`
	DirectIngress     bool                   `yaml:"direct_ingress"`
	ExposureMode      string                 `yaml:"exposure_mode,omitempty"`
	TlsPolicy         string                 `yaml:"tls_policy,omitempty"`
	IngressEndpointID string                 `yaml:"ingress_endpoint_id,omitempty"`
	DomainBindings    []string               `yaml:"domain_bindings,omitempty"`
	UpstreamHost      string                 `yaml:"upstream_host"`
	TraefikHTTPPort   int                    `yaml:"traefik_http_port"`
	Capabilities      map[string]interface{} `yaml:"capabilities,omitempty"`
	Region            string                 `yaml:"region,omitempty"`
}

type edgeClustersFile struct {
	Clusters map[string]edgeClusterEntry `yaml:"clusters"`
}

func loadEdgeClusters(ctx context.Context, c client.Client, ns, name string) (edgeClustersFile, error) {
	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
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

func saveEdgeClusters(ctx context.Context, c client.Client, ns, name string, f edgeClustersFile) error {
	if f.Clusters == nil {
		f.Clusters = map[string]edgeClusterEntry{}
	}
	raw, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "fleet-edge",
				"app.kubernetes.io/component": "cluster-registry",
			},
			Annotations: map[string]string{
				registry.AnnotationConsoleManaged: "true",
			},
		},
		Data: map[string]string{
			"clusters.yaml": string(raw),
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, c, cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data["clusters.yaml"] = string(raw)
		if cm.Annotations == nil {
			cm.Annotations = map[string]string{}
		}
		cm.Annotations[registry.AnnotationConsoleManaged] = "true"
		return nil
	})
	return err
}
