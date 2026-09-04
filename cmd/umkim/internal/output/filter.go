package output

import (
	"encoding/json"
	"fmt"
	"strings"
)

// NormalizeCapabilityRequirement maps user-facing tokens to API capability keys.
func NormalizeCapabilityRequirement(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "_")
	switch s {
	case "":
		return "", fmt.Errorf("empty capability requirement")
	case "gpu":
		return "gpu", nil
	case "kvm":
		return "kvm", nil
	case "ha", "ha_topology", "ha_cluster":
		return "ha_topology", nil
	case "rwx", "storage.rwx", "storage_rwx":
		return "storage.rwx", nil
	case "rwo", "storage.rwo", "storage_rwo":
		return "storage.rwo", nil
	default:
		if strings.Contains(s, ".") {
			return s, nil
		}
		return "", fmt.Errorf("unknown capability %q (use gpu, kvm, ha_topology, storage.rwx, storage.rwo)", s)
	}
}

// ParseRequireFlags expands --require values (comma-separated and/or repeated).
func ParseRequireFlags(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			key, err := NormalizeCapabilityRequirement(part)
			if err != nil {
				if strings.TrimSpace(part) == "" {
					continue
				}
				return nil, err
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
	}
	return out, nil
}

func clusterHasCapability(c map[string]interface{}, key string) bool {
	caps, _ := c["capabilities"].(map[string]interface{})
	if caps != nil {
		if boolField(caps, key) {
			return true
		}
	}
	labels, _ := c["labels"].(map[string]interface{})
	if labels != nil {
		switch key {
		case "gpu":
			return strField(labels, "fleet.capability/gpu") == "true"
		case "kvm":
			return strField(labels, "fleet.capability/kvm") == "true"
		case "ha_topology":
			return strField(labels, "fleet.topology/ha") == "true" ||
				strField(labels, "fleet.capability/ha_cluster") == "true"
		case "storage.rwx":
			return strField(labels, "fleet.capability/storage.rwx") == "true"
		case "storage.rwo":
			return strField(labels, "fleet.capability/storage.rwo") == "true"
		}
	}
	return false
}

// FilterClustersJSON keeps clusters that have every required capability (AND).
// When readyOnly is true, also requires provisioning_phase Ready (or available).
func FilterClustersJSON(raw []byte, require []string, readyOnly bool) ([]byte, error) {
	var body struct {
		Clusters []map[string]interface{} `json:"clusters"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(body.Clusters))
	for _, c := range body.Clusters {
		if readyOnly {
			phase := clusterPhase(c)
			if phase != "Ready" && !boolField(c, "available") {
				continue
			}
		}
		ok := true
		for _, key := range require {
			if !clusterHasCapability(c, key) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, c)
		}
	}
	body.Clusters = out
	return json.Marshal(body)
}

func WriteClusterCandidates(raw []byte, format Format) error {
	if format == FormatJSON {
		WriteJSON(raw)
		return nil
	}
	var body struct {
		Clusters []map[string]interface{} `json:"clusters"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	printTable(
		[]string{"NAME", "SITE", "PHASE", "READY", "GPU", "KVM", "HA", "RWX", "RWO"},
		body.Clusters,
		func(c map[string]interface{}) []string {
			return []string{
				strField(c, "name"),
				strField(c, "site"),
				clusterPhase(c),
				yesNo(boolField(c, "available")),
				yesNo(clusterHasCapability(c, "gpu")),
				yesNo(clusterHasCapability(c, "kvm")),
				yesNo(clusterHasCapability(c, "ha_topology")),
				yesNo(clusterHasCapability(c, "storage.rwx")),
				yesNo(clusterHasCapability(c, "storage.rwo")),
			}
		},
	)
	return nil
}
