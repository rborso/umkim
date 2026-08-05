package quota

import (
	"encoding/json"
	"testing"
)

func TestIsSharedPoolInstance(t *testing.T) {
	cases := []struct {
		name string
		ls   map[string]interface{}
		want bool
	}{
		{"missing pool", map[string]interface{}{"fleet.org": "acme"}, true},
		{"shared", map[string]interface{}{"fleet.org": "acme", "fleet.placement/pool": "shared"}, true},
		{"dedicated", map[string]interface{}{"fleet.org": "acme", "fleet.placement/pool": "dedicated"}, false},
		{"empty selector", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]interface{}{}
			if tc.ls != nil {
				payload["label_selector"] = tc.ls
			}
			if got := isSharedPoolInstance(payload); got != tc.want {
				t.Fatalf("isSharedPoolInstance = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExtractOrgAndPool(t *testing.T) {
	payload := map[string]interface{}{
		"label_selector": map[string]interface{}{
			"fleet.org":            "acme",
			"fleet.placement/pool": "dedicated",
		},
	}
	if org := extractOrg(payload); org != "acme" {
		t.Fatalf("extractOrg = %q", org)
	}
	if pool := extractPlacementPool(payload); pool != "dedicated" {
		t.Fatalf("extractPlacementPool = %q", pool)
	}

	nested := map[string]interface{}{
		"spec": map[string]interface{}{
			"label_selector": map[string]interface{}{
				"fleet.org":            "beta",
				"fleet.placement/pool": "shared",
			},
		},
	}
	if org := extractOrg(nested); org != "beta" {
		t.Fatalf("nested extractOrg = %q", org)
	}
}

func TestExtractResourceRequest(t *testing.T) {
	raw := `{
		"user_values": [
			{"path": "workload.replicas", "value": 2},
			{"path": "resources.cpu.min", "value": "500m"},
			{"path": "resources.memory.min", "value": "1Gi"},
			{"path": "workload.gpu.count", "value": 1}
		]
	}`
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	cpu, mem, gpu := extractResourceRequest(payload)
	if cpu != 1000 {
		t.Fatalf("cpu = %d, want 1000", cpu)
	}
	if mem != 2*1024*1024*1024 {
		t.Fatalf("mem = %d", mem)
	}
	if gpu != 1 {
		t.Fatalf("gpu = %d, want 1", gpu)
	}
}

func TestExtractResourceRequestChargesMaxOfRequestAndLimit(t *testing.T) {
	raw := `{
		"spec": {
			"user_values": [
				{"path": "resources.cpu.min", "value": "1m"},
				{"path": "resources.cpu.max", "value": "2000m"},
				{"path": "resources.memory.min", "value": "1MB"},
				{"path": "resources.memory.max", "value": "2Gi"}
			]
		}
	}`
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	cpu, mem, _ := extractResourceRequest(payload)
	if cpu != 2000 {
		t.Fatalf("cpu = %d, want 2000 (limit)", cpu)
	}
	if mem != 2*1024*1024*1024 {
		t.Fatalf("mem = %d, want 2Gi (limit)", mem)
	}
}
