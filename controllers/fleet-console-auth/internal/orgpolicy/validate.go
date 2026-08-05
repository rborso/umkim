package orgpolicy

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	labelOrg  = "fleet.org"
	labelPool = "fleet.pool"
	poolDedicated = "dedicated"
	poolShared    = "shared"
	placementPool = "fleet.placement/pool"
)

// ValidateMutation checks that an org-admin policy targets only their dedicated resources.
func ValidateMutation(orgSlug, policyID string, body []byte) error {
	orgSlug = strings.TrimSpace(orgSlug)
	if orgSlug == "" {
		return fmt.Errorf("org scope required")
	}
	if err := validatePrefixedID(orgSlug, policyID); err != nil {
		return err
	}
	if len(body) == 0 {
		// DELETE has no body; id prefix is enough.
		return nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("invalid policy JSON: %w", err)
	}
	match := matchLabels(payload, "cluster_requirements")
	if match[labelOrg] != orgSlug {
		return fmt.Errorf("cluster_requirements.matchLabels[%q] must be %q", labelOrg, orgSlug)
	}
	if match[labelPool] != poolDedicated {
		return fmt.Errorf("cluster_requirements.matchLabels[%q] must be %q", labelPool, poolDedicated)
	}
	if match[labelPool] == poolShared || match[placementPool] == poolShared {
		return fmt.Errorf("shared pool is not allowed in org-admin policies")
	}
	ls := stringMap(payload["label_selector"])
	if ls[labelOrg] != orgSlug {
		return fmt.Errorf("label_selector[%q] must be %q", labelOrg, orgSlug)
	}
	if ls[labelPool] == poolShared || ls[placementPool] == poolShared {
		return fmt.Errorf("shared pool is not allowed in org-admin policies")
	}
	return nil
}

func validatePrefixedID(orgSlug, policyID string) error {
	policyID = strings.TrimSpace(policyID)
	if policyID == "" {
		return fmt.Errorf("policy id required")
	}
	prefix := orgSlug + "-"
	if !strings.HasPrefix(policyID, prefix) || len(policyID) <= len(prefix) {
		return fmt.Errorf("policy id %q must start with %q", policyID, prefix)
	}
	return nil
}

// BelongsToOrg reports whether a listed policy is visible to the org.
func BelongsToOrg(orgSlug string, obj map[string]interface{}) bool {
	orgSlug = strings.TrimSpace(orgSlug)
	if orgSlug == "" {
		return false
	}
	id := policyID(obj)
	if id != "" && strings.HasPrefix(id, orgSlug+"-") {
		return true
	}
	match := matchLabels(obj, "cluster_requirements")
	if match[labelOrg] == orgSlug {
		return true
	}
	ls := stringMap(obj["label_selector"])
	return ls[labelOrg] == orgSlug
}

func policyID(obj map[string]interface{}) string {
	for _, k := range []string{"id", "uid", "name"} {
		if v, ok := obj[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func matchLabels(payload map[string]interface{}, key string) map[string]string {
	req, _ := payload[key].(map[string]interface{})
	if req == nil {
		return map[string]string{}
	}
	return stringMap(req["matchLabels"])
}

func stringMap(v interface{}) map[string]string {
	m, ok := v.(map[string]interface{})
	if !ok || m == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		out[k] = fmt.Sprint(val)
	}
	return out
}
