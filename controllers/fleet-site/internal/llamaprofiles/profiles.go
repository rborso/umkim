package llamaprofiles

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

//go:embed data/values-fleet-runtime.yaml
var runtimeYAML []byte

//go:embed data/profiles/*.yaml
var profileFS embed.FS

// ProfileMeta describes a built-in llama-cpp-helm values profile.
type ProfileMeta struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

var profileDescriptions = map[string]string{
	"qwen35-2b-ud-4g":    "~4GB GPU: Unsloth Qwen3.5-2B-UD Q8_K_XL, hub LiteLLM alias Qwen3.5-2B-UD",
	"qwen35-2b-ud-small": "Small-RAM guests (~4GiB VM): same model with reduced memory requests",
}

// List returns known profile ids sorted.
func List() []ProfileMeta {
	ids := profileIDs()
	out := make([]ProfileMeta, 0, len(ids))
	for _, id := range ids {
		out = append(out, ProfileMeta{
			ID:          id,
			Description: profileDescriptions[id],
		})
	}
	return out
}

// ValidProfileID reports whether id is a known profile.
func ValidProfileID(id string) bool {
	id = strings.TrimSpace(id)
	for _, p := range profileIDs() {
		if p == id {
			return true
		}
	}
	return false
}

func profileIDs() []string {
	entries, err := profileFS.ReadDir("data/profiles")
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(ids)
	return ids
}

// MergeProfileYAML returns Helm values YAML for runtime + profile + Harbor ghcr-proxy image.
func MergeProfileYAML(profileID, imageRepository string) (string, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return "", fmt.Errorf("profile id is required")
	}
	raw, err := profileFS.ReadFile("data/profiles/" + profileID + ".yaml")
	if err != nil {
		return "", fmt.Errorf("unknown profile %q", profileID)
	}
	return MergeProfileYAMLBytes(raw, imageRepository)
}

// MergeProfileYAMLBytes merges fleet runtime values, profile overlay YAML, and optional image repository.
func MergeProfileYAMLBytes(profileOverlay []byte, imageRepository string) (string, error) {
	runtime := map[string]any{}
	if err := yaml.Unmarshal(runtimeYAML, &runtime); err != nil {
		return "", err
	}
	prof := map[string]any{}
	if len(profileOverlay) > 0 {
		if err := yaml.Unmarshal(profileOverlay, &prof); err != nil {
			return "", err
		}
	}
	merged := deepMerge(runtime, prof)
	if strings.TrimSpace(imageRepository) != "" {
		img, _ := merged["image"].(map[string]any)
		if img == nil {
			img = map[string]any{}
		}
		img["repository"] = imageRepository
		img["tag"] = "server-cuda"
		img["pullPolicy"] = "IfNotPresent"
		merged["image"] = img
	}
	b, err := yaml.Marshal(merged)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// EmbeddedProfileYAML returns the built-in profile overlay for id, if any.
func EmbeddedProfileYAML(id string) ([]byte, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("profile id is required")
	}
	return profileFS.ReadFile("data/profiles/" + id + ".yaml")
}

func deepMerge(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if av, ok := out[k]; ok {
			am, ok1 := av.(map[string]any)
			bm, ok2 := v.(map[string]any)
			if ok1 && ok2 {
				out[k] = deepMerge(am, bm)
				continue
			}
		}
		out[k] = v
	}
	return out
}
