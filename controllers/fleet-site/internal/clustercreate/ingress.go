package clustercreate

import (
	"context"

	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type edgeClustersFile struct {
	Clusters map[string]edgeClusterEntry `yaml:"clusters"`
}

type edgeClusterEntry struct {
	Site      string `yaml:"site"`
	IngressID string `yaml:"ingressId"`
}

func existingIngressBindings(ctx context.Context, c client.Client, cfg config.Config, siteID, skipCluster string) (map[string][]string, error) {
	edge, err := loadEdgeClusters(ctx, c, cfg.EdgeClustersCMNamespace, cfg.EdgeClustersCMName)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for name, ent := range edge.Clusters {
		if name == skipCluster || ent.IngressID == "" {
			continue
		}
		if ent.Site != "" && ent.Site != siteID {
			continue
		}
		out[ent.IngressID] = append(out[ent.IngressID], name)
	}
	return out, nil
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
	var out edgeClustersFile
	if err := yaml.Unmarshal([]byte(raw), &out); err != nil {
		return edgeClustersFile{}, err
	}
	if out.Clusters == nil {
		out.Clusters = map[string]edgeClusterEntry{}
	}
	return out, nil
}
