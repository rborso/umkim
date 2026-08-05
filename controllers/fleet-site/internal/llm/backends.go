package llm

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/dcm"
	"github.com/rborso/umkim/controllers/fleet-site/internal/workload"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const llmWorkloadProfile = "llm"

type edgeClusterEntry struct {
	UpstreamHost string `yaml:"upstream_host"`
}

type edgeClustersFile struct {
	Clusters map[string]edgeClusterEntry `yaml:"clusters"`
}

// SyncBackendsFromDCM discovers llama-cpp-helm instances and resolves OpenAI-compatible API bases.
func SyncBackendsFromDCM(ctx context.Context, k8s client.Client, edgeNS, edgeCM, dcmURL string, existing []fleetv1alpha1.FleetLLMBackend) ([]fleetv1alpha1.FleetLLMBackend, error) {
	dcmClient := dcm.NewClient(dcmURL)
	instances, err := dcmClient.ListCatalogItemInstances(ctx)
	if err != nil {
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

	for _, inst := range instances {
		if inst.Spec.LabelSelector["fleet.workload/profile"] != llmWorkloadProfile {
			continue
		}
		catalogID := inst.Spec.CatalogItemID
		if catalogID == "" {
			catalogID = inst.Spec.CatalogItemName
		}
		if catalogID != "llama-cpp-helm" {
			continue
		}
		targets, err := dcmClient.GetPlacementTargets(ctx, inst.InstanceID())
		if err != nil {
			continue
		}
		model := dcm.UserValueHelmField(inst.Spec.UserValues, "model")
		if model == "" {
			model = dcm.UserValueHelmField(inst.Spec.UserValues, "hfRepo")
			if model == "" {
				model = catalogID + "-" + inst.InstanceID()
			}
		}
		port := dcm.UserValueHelmInt(inst.Spec.UserValues, 8080, "service", "port")
		for _, t := range targets.Targets {
			if !t.Healthy && t.Status != "" && t.Status != "active" {
				continue
			}
			cluster := workload.ProviderToCluster(t.Provider)
			upstream := edge[cluster]
			if upstream == "" {
				continue
			}
			apiBase := fmt.Sprintf("http://%s:%d/v1", upstream, port)
			byModel[model] = fleetv1alpha1.FleetLLMBackend{Model: model, APIBase: apiBase}
		}
	}

	out := make([]fleetv1alpha1.FleetLLMBackend, 0, len(byModel))
	for _, b := range byModel {
		out = append(out, b)
	}
	return out, nil
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
