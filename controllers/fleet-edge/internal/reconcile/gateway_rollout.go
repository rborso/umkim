package reconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/rborso/umkim/controllers/fleet-edge/internal/traefik"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const gatewayFleetRoutesChecksumKey = "checksum/fleet-traefik-fleet-routes"

func (r *RouteReconciler) fleetRoutesYAMLFromCM(ctx context.Context, ns, name, key string) []byte {
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	var cm corev1.ConfigMap
	cmKey := types.NamespacedName{Namespace: ns, Name: name}
	if err := reader.Get(ctx, cmKey, &cm); err != nil || cm.Data == nil {
		return nil
	}
	return []byte(cm.Data[key])
}

func (r *RouteReconciler) rolloutGatewayForFleetRoutes(ctx context.Context, prevRoutesYAML, routesYAML []byte) error {
	key := r.gatewayDeploymentKey()

	var dep appsv1.Deployment
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	if err := reader.Get(ctx, key, &dep); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	liveYAML := r.fleetRoutesYAMLFromCM(ctx, r.Config.FleetRoutesCMNS, r.Config.FleetRoutesCMName, r.Config.FleetRoutesCMKey)
	if len(liveYAML) > 0 {
		routesYAML = liveYAML
	}

	sum := sha256.Sum256(routesYAML)
	checksum := hex.EncodeToString(sum[:])
	existing := ""
	if dep.Spec.Template.Annotations != nil {
		existing = dep.Spec.Template.Annotations[gatewayFleetRoutesChecksumKey]
	}
	// Skip rollout when the live gateway already carries this fleet-routes checksum.
	if existing == checksum {
		return nil
	}
	checksumChanged := existing != checksum

	apply := func(ctx context.Context) error {
		var current appsv1.Deployment
		if err := r.Client.Get(ctx, key, &current); err != nil {
			return err
		}
		return r.patchGatewayFleetRoutesChecksum(ctx, &current, checksum)
	}

	// During ACME issuance the gateway must stay on :443 for Let's Encrypt TLS-ALPN.
	// Route reconciles still update the ConfigMap; issuance performs one controlled rollout.
	if checksumChanged && (r.acme == nil || !r.acme.Issuing()) {
		if err := apply(ctx); err != nil {
			return err
		}
	}

	if r.Config.ACMEEnabled && len(r.Config.FleetRoutesCMKey) > 0 && r.acme != nil && checksumChanged {
		pending := r.discoverPending(ctx, prevRoutesYAML, routesYAML)
		if len(pending) > 0 {
			r.acme.enqueue(pending...)
		}
	}
	return nil
}

// enqueuePendingACME exposes queueing for tests.
func (r *RouteReconciler) enqueuePendingACME(ctx context.Context, prevRoutesYAML, routesYAML []byte) {
	if r.acme == nil || !r.Config.ACMEEnabled {
		return
	}
	pending := discoverPendingHostnames(ctx, effectiveGatewayProbeHost(r.Config), prevRoutesYAML, routesYAML)
	if len(pending) > 0 {
		r.acme.enqueue(pending...)
	}
}

// HostnamesPendingProduction is used by tests and diagnostics.
func HostnamesPendingProduction(ctx context.Context, probeHost string, routesYAML []byte) ([]string, error) {
	hosts, err := traefik.HostnamesForResolver(routesYAML, traefik.CertResolverProduction)
	if err != nil {
		return nil, err
	}
	var pending []string
	for _, h := range hosts {
		if hostnameNeedsProduction(ctx, probeHost, h) {
			pending = append(pending, h)
		}
	}
	return pending, nil
}
