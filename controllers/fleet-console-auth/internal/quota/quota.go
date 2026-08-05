package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CheckCreate validates org shared-pool quota before catalog instance create.
func CheckCreate(ctx context.Context, k8s client.Client, ns, dcmUpstream string, body []byte, orgScopes []string) error {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	org := extractOrg(payload)
	if org == "" && len(orgScopes) == 1 {
		org = orgScopes[0]
	}
	if org == "" {
		return fmt.Errorf("fleet.org label required for instance create")
	}
	allowed := false
	for _, o := range orgScopes {
		if o == org {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("forbidden org %q", org)
	}
	var fleetOrg authv1.FleetOrg
	if err := k8s.Get(ctx, types.NamespacedName{Namespace: ns, Name: org}, &fleetOrg); err != nil {
		return fmt.Errorf("org %q not found", org)
	}
	// Dedicated-pool instances are not counted against sharedQuota.
	if !isSharedPoolInstance(payload) {
		return nil
	}
	if fleetOrg.Spec.SharedQuota == nil {
		return nil
	}
	reqCPU, reqMem, reqGPU := extractResourceRequest(payload)
	if reqCPU == 0 && reqMem == 0 && reqGPU == 0 {
		return nil
	}
	usedCPU, usedMem, usedGPU, err := sumOrgSharedUsage(ctx, dcmUpstream, org)
	if err != nil {
		return err
	}
	if fleetOrg.Spec.SharedQuota.CPUMilli > 0 && usedCPU+reqCPU > fleetOrg.Spec.SharedQuota.CPUMilli {
		return fmt.Errorf("org %q shared CPU quota exceeded", org)
	}
	if fleetOrg.Spec.SharedQuota.MemoryBytes > 0 && usedMem+reqMem > fleetOrg.Spec.SharedQuota.MemoryBytes {
		return fmt.Errorf("org %q shared memory quota exceeded", org)
	}
	if fleetOrg.Spec.SharedQuota.GPUCount > 0 && usedGPU+reqGPU > fleetOrg.Spec.SharedQuota.GPUCount {
		return fmt.Errorf("org %q shared GPU quota exceeded", org)
	}
	return nil
}

func extractOrg(payload map[string]interface{}) string {
	if ls := labelSelectorOf(payload); ls != nil {
		if org, ok := ls["fleet.org"].(string); ok {
			return org
		}
	}
	return ""
}

func extractPlacementPool(payload map[string]interface{}) string {
	ls := labelSelectorOf(payload)
	if ls == nil {
		return ""
	}
	pool, _ := ls["fleet.placement/pool"].(string)
	return pool
}

// isSharedPoolInstance reports whether the instance counts toward sharedQuota.
// Missing fleet.placement/pool is treated as shared (wizard default / legacy).
func isSharedPoolInstance(payload map[string]interface{}) bool {
	pool := extractPlacementPool(payload)
	return pool == "" || pool == "shared"
}

func labelSelectorOf(payload map[string]interface{}) map[string]interface{} {
	if ls, ok := payload["label_selector"].(map[string]interface{}); ok {
		return ls
	}
	if spec, ok := payload["spec"].(map[string]interface{}); ok {
		if ls, ok := spec["label_selector"].(map[string]interface{}); ok {
			return ls
		}
	}
	return nil
}

func userValuesOf(payload map[string]interface{}) []interface{} {
	if uv, ok := payload["user_values"].([]interface{}); ok {
		return uv
	}
	if spec, ok := payload["spec"].(map[string]interface{}); ok {
		if uv, ok := spec["user_values"].([]interface{}); ok {
			return uv
		}
	}
	return nil
}

// extractResourceRequest returns billed shared-pool usage for one instance.
// CPU/memory charge max(request, limit) so tiny requests + huge limits cannot bypass quota.
func extractResourceRequest(payload map[string]interface{}) (cpuMilli int64, memBytes int64, gpuCount int64) {
	uv := userValuesOf(payload)
	if len(uv) == 0 {
		return 0, 0, 0
	}
	var replicas int64 = 1
	var cpuReq, cpuLim, memReq, memLim int64
	for _, entry := range uv {
		m, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		path, _ := m["path"].(string)
		val := m["value"]
		switch path {
		case "workload.replicas", "values.replicaCount":
			replicas = toInt64(val)
			if replicas < 1 {
				replicas = 1
			}
		case "workload.gpu.count":
			gpuCount = toInt64(val)
		case "resources.cpu.min":
			cpuReq = parseCPU(val)
		case "resources.cpu.max":
			cpuLim = parseCPU(val)
		case "resources.memory.min":
			memReq = parseMemory(val)
		case "resources.memory.max":
			memLim = parseMemory(val)
		}
	}
	cpuMilli = maxInt64(cpuReq, cpuLim) * replicas
	memBytes = maxInt64(memReq, memLim) * replicas
	if gpuCount == 0 {
		gpuCount = replicas
	}
	return cpuMilli, memBytes, gpuCount
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func sumOrgSharedUsage(ctx context.Context, dcmUpstream, org string) (cpuMilli, memBytes, gpuCount int64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(dcmUpstream, "/")+"/api/v1alpha1/catalog-item-instances", nil)
	if err != nil {
		return 0, 0, 0, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, 0, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, 0, 0, err
	}
	results, _ := payload["results"].([]interface{})
	for _, item := range results {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if !isSharedPoolInstance(obj) {
			continue
		}
		itemOrg := ""
		if ls := labelSelectorOf(obj); ls != nil {
			itemOrg, _ = ls["fleet.org"].(string)
		}
		if itemOrg != org {
			continue
		}
		c, m, g := extractResourceRequest(obj)
		cpuMilli += c
		memBytes += m
		gpuCount += g
	}
	return cpuMilli, memBytes, gpuCount, nil
}

func toInt64(v interface{}) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	case string:
		var n int64
		fmt.Sscanf(t, "%d", &n)
		return n
	default:
		return 1
	}
}

func parseCPU(v interface{}) int64 {
	s := fmt.Sprint(v)
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "m") {
		var n int64
		fmt.Sscanf(strings.TrimSuffix(s, "m"), "%d", &n)
		return n
	}
	var cores int64
	fmt.Sscanf(s, "%d", &cores)
	return cores * 1000
}

func parseMemory(v interface{}) int64 {
	s := strings.ToUpper(fmt.Sprint(v))
	var n int64
	switch {
	case strings.HasSuffix(s, "GI"):
		fmt.Sscanf(strings.TrimSuffix(s, "GI"), "%d", &n)
		return n * 1024 * 1024 * 1024
	case strings.HasSuffix(s, "GB"):
		fmt.Sscanf(strings.TrimSuffix(s, "GB"), "%d", &n)
		return n * 1000 * 1000 * 1000
	case strings.HasSuffix(s, "MI"):
		fmt.Sscanf(strings.TrimSuffix(s, "MI"), "%d", &n)
		return n * 1024 * 1024
	case strings.HasSuffix(s, "MB"):
		fmt.Sscanf(strings.TrimSuffix(s, "MB"), "%d", &n)
		return n * 1000 * 1000
	default:
		fmt.Sscanf(s, "%d", &n)
		return n
	}
}

// UpdateOrgUsage recomputes FleetOrg.status.sharedUsage from DCM instances.
func UpdateOrgUsage(ctx context.Context, k8s client.Client, ns, dcmUpstream, org string) error {
	cpu, mem, gpu, err := sumOrgSharedUsage(ctx, dcmUpstream, org)
	if err != nil {
		return err
	}
	var fleetOrg authv1.FleetOrg
	if err := k8s.Get(ctx, types.NamespacedName{Namespace: ns, Name: org}, &fleetOrg); err != nil {
		return err
	}
	if fleetOrg.Status.SharedUsage == nil {
		fleetOrg.Status.SharedUsage = &authv1.OrgResourceUsage{}
	}
	fleetOrg.Status.SharedUsage.CPUMilli = cpu
	fleetOrg.Status.SharedUsage.MemoryBytes = mem
	fleetOrg.Status.SharedUsage.GPUCount = gpu
	return k8s.Status().Update(ctx, &fleetOrg)
}
