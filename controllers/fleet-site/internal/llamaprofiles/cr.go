package llamaprofiles

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MergeForProfile returns merged Helm values YAML for a FleetLlamaProfile CR.
func MergeForProfile(p *fleetv1alpha1.FleetLlamaProfile, imageRepository string) (string, error) {
	overlay := []byte(strings.TrimSpace(p.Spec.ProfileYAML))
	if len(overlay) == 0 && p.Spec.System {
		raw, err := EmbeddedProfileYAML(p.Name)
		if err != nil {
			return "", err
		}
		overlay = raw
	}
	return MergeProfileYAMLBytes(overlay, imageRepository)
}

// MergedValuesHash fingerprints merged values for status cache.
func MergedValuesHash(valuesYAML string) string {
	sum := sha256.Sum256([]byte(valuesYAML))
	return hex.EncodeToString(sum[:8])
}

// EnsureSystemProfiles seeds built-in profiles as system CRs when missing.
func EnsureSystemProfiles(ctx context.Context, c client.Client, ns string) error {
	for _, id := range profileIDs() {
		var existing fleetv1alpha1.FleetLlamaProfile
		err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: id}, &existing)
		if err == nil {
			continue
		}
		if !apierrors.IsNotFound(err) {
			return err
		}
		raw, err := EmbeddedProfileYAML(id)
		if err != nil {
			return err
		}
		desc := profileDescriptions[id]
		cr := &fleetv1alpha1.FleetLlamaProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      id,
				Namespace: ns,
				Labels: map[string]string{
					"fleet.umkim.com/llama-profile-system": "true",
				},
			},
			Spec: fleetv1alpha1.FleetLlamaProfileSpec{
				Description:  desc,
				ProfileYAML:  string(raw),
				Chart:        fleetv1alpha1.DefaultLlamaCppChart,
				ChartVersion: fleetv1alpha1.DefaultLlamaCppChartVersion,
				System:       true,
			},
		}
		if err := c.Create(ctx, cr); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}

// ListProfileMetasFromCRs returns profile list entries from CRs plus legacy embed ids not yet in cluster.
func ListProfileMetasFromCRs(items []fleetv1alpha1.FleetLlamaProfile) []ProfileMeta {
	seen := map[string]struct{}{}
	out := make([]ProfileMeta, 0, len(items))
	for _, p := range items {
		seen[p.Name] = struct{}{}
		out = append(out, ProfileMeta{
			ID:          p.Name,
			Description: strings.TrimSpace(p.Spec.Description),
		})
	}
	for _, id := range profileIDs() {
		if _, ok := seen[id]; ok {
			continue
		}
		out = append(out, ProfileMeta{
			ID:          id,
			Description: profileDescriptions[id],
		})
	}
	return out
}

// ResolveProfile loads a profile by name from the console namespace or falls back to embedded id.
func ResolveProfile(ctx context.Context, c client.Client, cfg config.Config, name string) (*fleetv1alpha1.FleetLlamaProfile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("profile name is required")
	}
	ns := cfg.ConsoleNamespace
	var p fleetv1alpha1.FleetLlamaProfile
	err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &p)
	if err == nil {
		return &p, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}
	if !ValidProfileID(name) {
		return nil, fmt.Errorf("profile %q not found", name)
	}
	raw, err := EmbeddedProfileYAML(name)
	if err != nil {
		return nil, err
	}
	return &fleetv1alpha1.FleetLlamaProfile{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: fleetv1alpha1.FleetLlamaProfileSpec{
			Description:  profileDescriptions[name],
			ProfileYAML:  string(raw),
			Chart:        fleetv1alpha1.DefaultLlamaCppChart,
			ChartVersion: fleetv1alpha1.DefaultLlamaCppChartVersion,
			System:       true,
		},
	}, nil
}
