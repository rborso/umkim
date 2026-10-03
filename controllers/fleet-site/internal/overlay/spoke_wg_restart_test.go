package overlay

import "testing"

func TestSpokeWGRestartManifestWork(t *testing.T) {
	mw := SpokeWGRestartManifestWork("lab-ext02", "fleet-overlay-wg-restart-lab-ext02-1")
	if mw.GetNamespace() != "lab-ext02" {
		t.Fatalf("namespace: %s", mw.GetNamespace())
	}
	manifests, found, err := unstructuredNestedSlice(mw.Object, "spec", "workload", "manifests")
	if err != nil || !found || len(manifests) != 1 {
		t.Fatalf("manifests: found=%v err=%v len=%d", found, err, len(manifests))
	}
	job, ok := manifests[0].(map[string]interface{})
	if !ok || job["kind"] != "Job" {
		t.Fatalf("expected Job manifest, got %T", manifests[0])
	}
}

func unstructuredNestedSlice(obj map[string]interface{}, fields ...string) ([]interface{}, bool, error) {
	cur := obj
	for i, f := range fields {
		if i == len(fields)-1 {
			v, ok := cur[f]
			if !ok {
				return nil, false, nil
			}
			s, ok := v.([]interface{})
			return s, ok, nil
		}
		next, ok := cur[f].(map[string]interface{})
		if !ok {
			return nil, false, nil
		}
		cur = next
	}
	return nil, false, nil
}
