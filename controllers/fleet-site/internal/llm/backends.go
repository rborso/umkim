package llm

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type edgeClusterEntry struct {
	UpstreamHost string `yaml:"upstream_host"`
}

type edgeClustersFile struct {
	Clusters map[string]edgeClusterEntry `yaml:"clusters"`
}

// SyncBackendsFromFleetApps discovers llama-cpp Helm releases and resolves OpenAI-compatible API bases.
func SyncBackendsFromFleetApps(ctx context.Context, k8s client.Client, edgeNS, edgeCM string, existing []fleetv1alpha1.FleetLLMBackend) ([]fleetv1alpha1.FleetLLMBackend, error) {
	var apps fleetv1alpha1.FleetAppList
	if err := k8s.List(ctx, &apps); err != nil {
		return existing, err
	}
	edge, err := loadEdgeUpstream(ctx, k8s, edgeNS, edgeCM)
	if err != nil {
		return existing, err
	}

	byModel := map[string]fleetv1alpha1.FleetLLMBackend{}
	for _, b := range existing {
		if b.Model != "" && b.APIBase != "" {
			byModel[b.Model] = b
		}
	}

	for _, app := range apps.Items {
		if !IsLlamaChart(app.Spec.Chart) {
			continue
		}
		if app.Status.Phase != fleetv1alpha1.AppPhaseReady && app.Status.Phase != "" {
			continue
		}
		cluster := strings.TrimSpace(app.Spec.ClusterName)
		upstream := edge[cluster]
		if upstream == "" {
			continue
		}
		vals := parseValues(app.Spec.ValuesYAML)
		model := stringVal(vals, "model")
		if model == "" {
			model = stringVal(vals, "hfRepo")
		}
		if model == "" {
			model = app.Name
		}
		port := intVal(vals, 8080, "service", "port")
		apiBase := fmt.Sprintf("http://%s:%d/v1", upstream, port)
		byModel[model] = fleetv1alpha1.FleetLLMBackend{Model: model, APIBase: apiBase}
	}

	out := make([]fleetv1alpha1.FleetLLMBackend, 0, len(byModel))
	for _, b := range byModel {
		out = append(out, b)
	}
	return out, nil
}

// IsLlamaChart reports whether a FleetApp chart name refers to llama-cpp-helm.
func IsLlamaChart(chart string) bool {
	c := strings.ToLower(strings.TrimSpace(chart))
	return strings.Contains(c, "llama-cpp")
}

func parseValues(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := yaml.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func stringVal(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func intVal(m map[string]any, def int, keys ...string) int {
	cur := any(m)
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return def
		}
		cur = mm[k]
	}
	switch n := cur.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return def
	}
}

func loadEdgeUpstream(ctx context.Context, c client.Client, ns, cmName string) (map[string]string, error) {
	if cmName == "" {
		cmName = "fleet-edge-domains"
	}
	f, err := loadEdgeClustersYAML(ctx, c, ns, cmName)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for name, entry := range f.Clusters {
		host := strings.TrimSpace(entry.UpstreamHost)
		if host != "" {
			out[name] = host
		}
	}
	return out, nil
}

func loadEdgeClustersYAML(ctx context.Context, c client.Client, ns, name string) (edgeClustersFile, error) {
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
