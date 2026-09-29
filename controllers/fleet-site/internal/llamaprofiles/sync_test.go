package llamaprofiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ensures embedded profile data stays aligned with deploy/llama-cpp-helm (repo root).
func TestEmbeddedProfilesMatchDeployTree(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "deploy", "llama-cpp-helm")
	if _, err := os.Stat(root); err != nil {
		t.Skip("deploy tree not present in this workspace")
	}
	runtimeDeploy, err := os.ReadFile(filepath.Join(root, "values-fleet-runtime.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// Compare normalized lines (ignore comments).
	if normalizeYAML(string(runtimeDeploy)) != normalizeYAML(string(runtimeYAML)) {
		t.Fatalf("embedded runtime YAML diverged from deploy/llama-cpp-helm/values-fleet-runtime.yaml")
	}
	profDeploy, err := os.ReadFile(filepath.Join(root, "profiles", "qwen35-2b-ud-4g.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	profEmbed, err := profileFS.ReadFile("data/profiles/qwen35-2b-ud-4g.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if normalizeYAML(string(profDeploy)) != normalizeYAML(string(profEmbed)) {
		t.Fatalf("embedded profile diverged from deploy/llama-cpp-helm/profiles/qwen35-2b-ud-4g.yaml")
	}
}

func normalizeYAML(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
