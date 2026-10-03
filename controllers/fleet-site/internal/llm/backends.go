package llm

import (
	"context"
	"fmt"
	"sort"
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

// DiscoverBackendsFromFleetApps returns LiteLLM backends for Ready registered llama-cpp FleetApps only.
func DiscoverBackendsFromFleetApps(ctx context.Context, k8s client.Client, edgeNS, edgeCM string) ([]fleetv1alpha1.FleetLLMBackend, error) {
	var apps fleetv1alpha1.FleetAppList
	if err := k8s.List(ctx, &apps); err != nil {
		return nil, err
	}
	edge, err := loadEdgeUpstream(ctx, k8s, edgeNS, edgeCM)
	if err != nil {
		return nil, err
	}
	return DiscoverBackendsFromApps(apps.Items, edge), nil
}

// SyncBackendsFromFleetApps discovers llama-cpp Helm releases and merges with existing manual backends.
func SyncBackendsFromFleetApps(ctx context.Context, k8s client.Client, edgeNS, edgeCM string, existing []fleetv1alpha1.FleetLLMBackend) ([]fleetv1alpha1.FleetLLMBackend, error) {
	auto, err := DiscoverBackendsFromFleetApps(ctx, k8s, edgeNS, edgeCM)
	if err != nil {
		return existing, err
	}
	return MergeBackends(existing, auto), nil
}

// DiscoverBackendsFromApps returns one LiteLLM backend per Ready llama-cpp FleetApp.
// The same model alias may appear multiple times (load balancing across clusters).
func DiscoverBackendsFromApps(apps []fleetv1alpha1.FleetApp, edge map[string]string) []fleetv1alpha1.FleetLLMBackend {
	var out []fleetv1alpha1.FleetLLMBackend
	seen := map[string]struct{}{}
	for _, app := range apps {
		if !IsLlamaChart(app.Spec.Chart) {
			continue
		}
		if !app.RegistersLLMBackend() {
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
		key := model + "\x00" + apiBase
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, fleetv1alpha1.FleetLLMBackend{Model: model, APIBase: apiBase})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return out[i].APIBase < out[j].APIBase
	})
	return out
}

// MergeBackends keeps manual FleetLLM backends that are not auto-discovered, then appends auto entries.
func MergeBackends(existing, auto []fleetv1alpha1.FleetLLMBackend) []fleetv1alpha1.FleetLLMBackend {
	autoKeys := map[string]struct{}{}
	autoBases := map[string]struct{}{}
	for _, b := range auto {
		if b.Model == "" || b.APIBase == "" {
			continue
		}
		autoKeys[b.Model+"\x00"+b.APIBase] = struct{}{}
		autoBases[b.APIBase] = struct{}{}
	}
	var out []fleetv1alpha1.FleetLLMBackend
	seen := map[string]struct{}{}
	for _, b := range existing {
		if b.Model == "" || b.APIBase == "" {
			continue
		}
		key := b.Model + "\x00" + b.APIBase
		if _, ok := autoKeys[key]; ok {
			continue
		}
		// Drop superseded llama aliases at the same spoke apiBase as a discovered backend.
		if _, onAutoBase := autoBases[b.APIBase]; onAutoBase {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, b)
	}
	for _, b := range auto {
		key := b.Model + "\x00" + b.APIBase
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, b)
	}
	return out
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
