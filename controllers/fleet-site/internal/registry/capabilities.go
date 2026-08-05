package registry

import (
	"strings"
)

// Capability keys used in API JSON (map keys under cluster.capabilities).
const (
	CapGPU          = "gpu"
	CapStorageRWO   = "storage.rwo"
	CapStorageRWX   = "storage.rwx"
	CapKVM          = "kvm"
	CapHATopology   = "ha_topology"
	CapHAStorage    = "ha_storage" // deprecated alias
	CapHACluster    = "ha_cluster" // deprecated alias
)

// CapabilitiesFromLabels parses OCM ManagedCluster fleet.* labels into API capability map.
func CapabilitiesFromLabels(labels map[string]string) map[string]bool {
	if labels == nil {
		labels = map[string]string{}
	}
	out := map[string]bool{
		CapGPU:        labelTrue(labels["fleet.capability/gpu"]),
		CapStorageRWO: labelTrue(labels["fleet.capability/storage.rwo"]),
		CapStorageRWX: labelTrue(labels["fleet.capability/storage.rwx"]),
		CapKVM:        labelTrue(labels["fleet.capability/kvm"]),
		CapHATopology: labelTrue(labels["fleet.topology/ha"]),
	}
	if !out[CapHATopology] {
		out[CapHATopology] = labelTrue(labels["fleet.capability/ha_cluster"])
	}
	if !out[CapStorageRWO] && !out[CapStorageRWX] {
		if labelTrue(labels["fleet.capability/ha_storage"]) {
			out[CapStorageRWO] = true
		}
	}
	out[CapHAStorage] = out[CapStorageRWO] || out[CapStorageRWX]
	if out[CapHATopology] {
		out[CapHACluster] = true
	}
	return out
}

// ApplyCapabilityPatch merges API capability keys onto OCM labels (including deprecated aliases).
func ApplyCapabilityPatch(labels map[string]string, caps map[string]bool) {
	if labels == nil {
		return
	}
	for k, v := range caps {
		switch k {
		case "external_access", "external_datacenter":
			continue
		case CapGPU, CapStorageRWO, CapStorageRWX, CapKVM, CapHAStorage, CapHACluster:
			labels["fleet.capability/"+k] = boolLabel(v)
		case CapHATopology:
			labels["fleet.topology/ha"] = boolLabel(v)
			labels["fleet.capability/ha_cluster"] = boolLabel(v)
		default:
			if strings.HasPrefix(k, "storage.") || k == "gpu" || k == "kvm" {
				labels["fleet.capability/"+k] = boolLabel(v)
			}
		}
	}
	syncCapabilityAliases(labels)
}

func syncCapabilityAliases(labels map[string]string) {
	rwo := labelTrue(labels["fleet.capability/storage.rwo"])
	rwx := labelTrue(labels["fleet.capability/storage.rwx"])
	if rwo || rwx {
		labels["fleet.capability/ha_storage"] = "true"
	} else {
		labels["fleet.capability/ha_storage"] = "false"
	}
	ha := labelTrue(labels["fleet.topology/ha"])
	if ha {
		labels["fleet.capability/ha_cluster"] = "true"
	}
}

func labelTrue(v string) bool {
	return v == "true" || v == "True"
}

func boolLabel(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// ClusterIngressEnabled reports whether the cluster exposes direct site ingress.
func ClusterIngressEnabled(cluster ClusterExposureInput, siteDefaultExposure string) bool {
	mode := EffectiveClusterExposureMode(cluster, siteDefaultExposure)
	return AllowsDirectSiteIngress(mode)
}
