package appchart

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

type chartMetaFile struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// MetaFromTGZ reads name and version from Chart.yaml inside a helm package.
func MetaFromTGZ(filename string) (name, version string, err error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	return MetaFromTGZReader(f)
}

func MetaFromTGZReader(r io.Reader) (name, version string, err error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", "", fmt.Errorf("not a gzip chart package: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", err
		}
		if hdr.FileInfo().IsDir() {
			continue
		}
		base := path.Base(hdr.Name)
		if base != "Chart.yaml" && base != "Chart.yml" {
			continue
		}
		// Prefer top-level <chart>/Chart.yaml (one slash after chart dir).
		rel := strings.TrimPrefix(hdr.Name, "./")
		if strings.Count(rel, "/") > 1 {
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(tr, 1<<20))
		if err != nil {
			return "", "", err
		}
		var meta chartMetaFile
		if err := yaml.Unmarshal(raw, &meta); err != nil {
			return "", "", fmt.Errorf("Chart.yaml: %w", err)
		}
		name = strings.ToLower(strings.TrimSpace(meta.Name))
		version = strings.TrimSpace(meta.Version)
		if name == "" {
			return "", "", fmt.Errorf("Chart.yaml is missing name")
		}
		return name, version, nil
	}
	return "", "", fmt.Errorf("Chart.yaml not found in package")
}

// MergeValuesYAML merges overlay onto base (overlay wins). Empty inputs are ignored.
func MergeValuesYAML(base, overlay string) (string, error) {
	base = strings.TrimSpace(base)
	overlay = strings.TrimSpace(overlay)
	if overlay == "" {
		return base, nil
	}
	if base == "" {
		return overlay, nil
	}
	var b, o interface{}
	if err := yaml.Unmarshal([]byte(base), &b); err != nil {
		return "", fmt.Errorf("library values: %w", err)
	}
	if err := yaml.Unmarshal([]byte(overlay), &o); err != nil {
		return "", fmt.Errorf("app values: %w", err)
	}
	merged := mergeYAML(b, o)
	out, err := yaml.Marshal(merged)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func mergeYAML(base, overlay interface{}) interface{} {
	bm, bok := asYAMLMap(base)
	om, ook := asYAMLMap(overlay)
	if bok && ook {
		out := map[string]interface{}{}
		for k, v := range bm {
			out[k] = v
		}
		for k, v := range om {
			if existing, ok := out[k]; ok {
				out[k] = mergeYAML(existing, v)
			} else {
				out[k] = v
			}
		}
		return out
	}
	if overlay != nil {
		return overlay
	}
	return base
}

func asYAMLMap(v interface{}) (map[string]interface{}, bool) {
	switch t := v.(type) {
	case map[string]interface{}:
		return t, true
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = val
		}
		return out, true
	default:
		return nil, false
	}
}
