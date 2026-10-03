package hubacme

import (
	"context"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const gatewayNS = "fleet-ingress"

// Profile is the hub Traefik ACME issuance mode (solver vs hostNetwork legacy).
type Profile struct {
	SolverEnabled bool
	LegacyScale   bool
	Preflight     bool
}

// Detect reads the live gateway (and optional gateway-acme) to choose ACME mode.
// hostNetwork legacy hubs have no gateway-acme; HA pod-network hubs use the solver.
func Detect(ctx context.Context, c client.Client) Profile {
	var gw appsv1.Deployment
	if err := c.Get(ctx, types.NamespacedName{Namespace: gatewayNS, Name: "gateway"}, &gw); err != nil {
		return legacyProfile()
	}
	replicas := int32(1)
	if gw.Spec.Replicas != nil && *gw.Spec.Replicas > 0 {
		replicas = *gw.Spec.Replicas
	}
	hostNet := gw.Spec.Template.Spec.HostNetwork
	solverExists := resourceExists(ctx, c, "Deployment", "gateway-acme")

	// Mirror hub Traefik HA: replicas>1 and not hostNetwork, plus gateway-acme.
	ha := replicas > 1 && !hostNet
	useSolver := ha && solverExists
	if useSolver {
		return Profile{
			SolverEnabled: true,
			LegacyScale:   false,
			Preflight:     true,
		}
	}
	return legacyProfile()
}

func legacyProfile() Profile {
	return Profile{
		SolverEnabled: false,
		LegacyScale:   true,
		Preflight:     false,
	}
}

func resourceExists(ctx context.Context, c client.Client, kind, name string) bool {
	switch kind {
	case "Deployment":
		var dep appsv1.Deployment
		err := c.Get(ctx, types.NamespacedName{Namespace: gatewayNS, Name: name}, &dep)
		return err == nil
	default:
		return false
	}
}

func boolYAML(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func yamlQuote(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// InventoryYAMLLines emits Ansible vars for fleet-edge-controller ACME profile.
func InventoryYAMLLines(p Profile) []string {
	return []string{
		"fleet_edge_acme_solver_enabled: " + boolYAML(p.SolverEnabled),
		"fleet_edge_acme_legacy_gateway_scale: " + boolYAML(p.LegacyScale),
		"fleet_edge_acme_preflight_enabled: " + boolYAML(p.Preflight),
	}
}
