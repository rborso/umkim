package llamaprofiles

import (
	"strings"
	"testing"
)

func TestMergeProfileYAML_qwen35(t *testing.T) {
	yaml, err := MergeProfileYAML("qwen35-2b-ud-4g", "registry.example:30443/ghcr-proxy/ggml-org/llama.cpp")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(yaml, "Qwen3.5-2B-UD") {
		t.Fatalf("missing model alias: %s", yaml)
	}
	if !strings.Contains(yaml, "enable_thinking") {
		t.Fatalf("missing chat template kwargs: %s", yaml)
	}
	if !strings.Contains(yaml, "jinja: true") {
		t.Fatalf("missing jinja: %s", yaml)
	}
	if !strings.Contains(yaml, "registry.example:30443/ghcr-proxy") {
		t.Fatalf("missing image repo: %s", yaml)
	}
}

func TestListIncludesQwenProfile(t *testing.T) {
	list := List()
	found := false
	for _, p := range list {
		if p.ID == "qwen35-2b-ud-4g" {
			found = true
		}
	}
	if !found {
		t.Fatalf("profiles: %+v", list)
	}
}
