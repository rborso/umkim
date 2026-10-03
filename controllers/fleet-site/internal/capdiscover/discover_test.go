package capdiscover

import (
	"encoding/json"
	"testing"
)

func TestResultUnmarshalAcceptsLegacyNumericStrings(t *testing.T) {
	var result Result
	err := json.Unmarshal([]byte(`{
		"nodes": [{"name": "n1", "gpu_count": "2", "capabilities": {"gpu": true}}],
		"cluster_resources": {"gpu.total": "2", "gpu.available": 1}
	}`), &result)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].GPUCount != 2 {
		t.Fatalf("unexpected nodes: %#v", result.Nodes)
	}
	if result.ClusterResources["gpu.total"] != 2 || result.ClusterResources["gpu.available"] != 1 {
		t.Fatalf("unexpected resources: %#v", result.ClusterResources)
	}
}

func TestFromAnnotations(t *testing.T) {
	r := FromAnnotations(map[string]string{
		AnnPhase: PhaseSucceeded,
		AnnMsg:   "ok",
		AnnCaps:  `{"kvm":true}`,
	})
	if r == nil || r.Phase != PhaseSucceeded || !r.ClusterCapabilities["kvm"] {
		t.Fatalf("got %#v", r)
	}
}

func TestWriteResultAnnotations(t *testing.T) {
	ann := map[string]string{}
	res := &Result{
		Phase:               PhaseSucceeded,
		Message:             "done",
		CompletedAt:         "2026-01-01T00:00:00Z",
		ClusterCapabilities: map[string]bool{"kvm": true},
		Nodes:               []NodeResult{{Name: "n1", Caps: map[string]bool{"kvm": true}}},
	}
	if err := WriteResultAnnotations(ann, res); err != nil {
		t.Fatal(err)
	}
	if ann[AnnPhase] != PhaseSucceeded || ann[AnnCaps] == "" || ann[AnnNodes] == "" {
		t.Fatalf("ann=%v", ann)
	}
}
