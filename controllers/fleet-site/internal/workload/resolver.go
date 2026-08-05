package workload

import (
	"context"
	"fmt"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/dcm"
)

// Resolved describes a deployable catalog instance workload for backup/restore.
type Resolved struct {
	InstanceID   string `json:"instance_id"`
	DisplayName  string `json:"display_name,omitempty"`
	ServiceType  string `json:"service_type,omitempty"`
	Cluster      string `json:"cluster"`
	Release      string `json:"release"`
	Namespace    string `json:"namespace"`
	HubHost      string `json:"hub_host,omitempty"`
	ActiveCluster string `json:"active_cluster,omitempty"`
	Deployed     bool   `json:"deployed"`
}

func ProviderToCluster(provider string) string {
	for _, prefix := range []string{"k8s-", "helm-", "olm-"} {
		if strings.HasPrefix(provider, prefix) {
			return strings.TrimPrefix(provider, prefix)
		}
	}
	return provider
}

func clusterFromPlacement(targets *dcm.PlacementTargetsResponse) string {
	if targets == nil {
		return ""
	}
	for _, t := range targets.Targets {
		if t.Role == "primary" && t.Provider != "" {
			return ProviderToCluster(t.Provider)
		}
	}
	for _, t := range targets.Targets {
		if t.Provider != "" && (t.Status == "active" || t.Healthy) {
			return ProviderToCluster(t.Provider)
		}
	}
	for _, t := range targets.Targets {
		if t.Provider != "" {
			return ProviderToCluster(t.Provider)
		}
	}
	return ""
}

// Resolve loads DCM instance + placement-targets into backup parameters.
func Resolve(ctx context.Context, client *dcm.Client, instanceID string) (*Resolved, error) {
	inst, err := client.GetCatalogItemInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if inst.IsClusterInstance() {
		return nil, fmt.Errorf("cluster instances cannot be backed up via instance backup API")
	}
	targets, err := client.GetPlacementTargets(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	cluster := dcm.UserValueString(inst.Spec.UserValues, "routing.cluster")
	if cluster == "" {
		cluster = clusterFromPlacement(targets)
	}
	release := dcm.UserValueString(inst.Spec.UserValues, "metadata.name")
	ns := dcm.UserValueString(inst.Spec.UserValues, "release.namespace")
	if ns == "" {
		ns = "default"
	}
	serviceType := inst.Spec.ServiceType
	if serviceType == "" {
		serviceType = "helm"
	}
	deployed := targets.TargetCount > 0 && cluster != "" && release != ""
	out := &Resolved{
		InstanceID:  instanceID,
		DisplayName: inst.DisplayName,
		ServiceType: serviceType,
		Cluster:     cluster,
		Release:     release,
		Namespace:   ns,
		Deployed:    deployed,
	}
	if !deployed {
		return out, fmt.Errorf("instance not deployed or missing placement")
	}
	return out, nil
}

// ApplyUserValuePatches returns a copy of user_values with patches applied.
func ApplyUserValuePatches(values []dcm.UserValue, patches []dcm.UserValue) []dcm.UserValue {
	out := make([]dcm.UserValue, 0, len(values)+len(patches))
	seen := map[string]struct{}{}
	for _, p := range patches {
		seen[p.Path] = struct{}{}
		out = append(out, p)
	}
	for _, uv := range values {
		if _, ok := seen[uv.Path]; ok {
			continue
		}
		out = append(out, uv)
	}
	return out
}
