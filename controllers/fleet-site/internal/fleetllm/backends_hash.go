package fleetllm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const AppliedBackendsHashAnnotation = "fleet.umkim.com/llm-applied-backends-hash"

func BackendsHash(backends []fleetv1alpha1.FleetLLMBackend) string {
	if len(backends) == 0 {
		return ""
	}
	cp := make([]fleetv1alpha1.FleetLLMBackend, len(backends))
	copy(cp, backends)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].Model != cp[j].Model {
			return cp[i].Model < cp[j].Model
		}
		return cp[i].APIBase < cp[j].APIBase
	})
	raw, _ := json.Marshal(cp)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func AppliedBackendsHash(fl *fleetv1alpha1.FleetLLM) string {
	if fl.Annotations == nil {
		return ""
	}
	return fl.Annotations[AppliedBackendsHashAnnotation]
}

func SetAppliedBackendsHash(ctx context.Context, c client.Client, fl *fleetv1alpha1.FleetLLM, backends []fleetv1alpha1.FleetLLMBackend) error {
	hash := BackendsHash(backends)
	cur := fl.DeepCopy()
	if cur.Annotations == nil {
		cur.Annotations = map[string]string{}
	}
	if cur.Annotations[AppliedBackendsHashAnnotation] == hash {
		return nil
	}
	cur.Annotations[AppliedBackendsHashAnnotation] = hash
	return c.Update(ctx, cur)
}
