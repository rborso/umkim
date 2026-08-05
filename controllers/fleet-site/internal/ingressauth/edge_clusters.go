package ingressauth

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AnsibleEdgeCluster mirrors fleet-edge-clusters fields needed for phase6 rendering.
type AnsibleEdgeCluster struct {
	DirectIngress bool   `yaml:"direct_ingress"`
	ExposureMode  string `yaml:"exposure_mode,omitempty"`
}

type ansibleEdgeClustersFile struct {
	Clusters map[string]AnsibleEdgeCluster `yaml:"clusters"`
}

// LoadEdgeClustersForAnsible reads fleet-edge-clusters for ingress-auth phase6 reconcile.
func LoadEdgeClustersForAnsible(ctx context.Context, c client.Client, ns, name string) (map[string]AnsibleEdgeCluster, error) {
	if ns == "" || name == "" {
		return map[string]AnsibleEdgeCluster{}, nil
	}
	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return map[string]AnsibleEdgeCluster{}, nil
		}
		return nil, err
	}
	raw := cm.Data["clusters.yaml"]
	if raw == "" {
		return map[string]AnsibleEdgeCluster{}, nil
	}
	var f ansibleEdgeClustersFile
	if err := yaml.Unmarshal([]byte(raw), &f); err != nil {
		return nil, fmt.Errorf("parse fleet-edge-clusters: %w", err)
	}
	if f.Clusters == nil {
		return map[string]AnsibleEdgeCluster{}, nil
	}
	return f.Clusters, nil
}
