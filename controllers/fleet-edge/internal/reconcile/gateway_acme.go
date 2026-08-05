package reconcile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/rborso/umkim/controllers/fleet-edge/internal/traefik"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	gatewayACMEPreflightWait     = 90 * time.Second
	gatewayACMEProductionWait    = 5 * time.Minute
	gatewayACMECertObtainPerHost = 45 * time.Second
	gatewayACMECertObtainWaitMax = 8 * time.Minute
)

type gatewayACMEProfile struct {
	replicas      int32
	hostNetwork   bool
	dnsPolicy     corev1.DNSPolicy
	strategyType  appsv1.DeploymentStrategyType
	rollingUpdate *appsv1.RollingUpdateDeployment
}

func gatewayProfile(dep appsv1.Deployment) gatewayACMEProfile {
	p := gatewayACMEProfile{replicas: 1, dnsPolicy: corev1.DNSClusterFirst, strategyType: appsv1.RollingUpdateDeploymentStrategyType}
	if dep.Spec.Replicas != nil {
		p.replicas = *dep.Spec.Replicas
	}
	p.hostNetwork = dep.Spec.Template.Spec.HostNetwork
	if dep.Spec.Template.Spec.DNSPolicy != "" {
		p.dnsPolicy = dep.Spec.Template.Spec.DNSPolicy
	}
	if dep.Spec.Strategy.Type != "" {
		p.strategyType = dep.Spec.Strategy.Type
	}
	if dep.Spec.Strategy.RollingUpdate != nil {
		ru := *dep.Spec.Strategy.RollingUpdate
		p.rollingUpdate = &ru
	}
	return p
}

func gatewayNeedsACMEMode(dep appsv1.Deployment) bool {
	p := gatewayProfile(dep)
	return p.replicas > 1 || !p.hostNetwork
}

func gatewayACMEWaitDuration(base time.Duration, hostCount int) time.Duration {
	if hostCount < 1 {
		hostCount = 1
	}
	wait := base + time.Duration(hostCount-1)*gatewayACMECertObtainPerHost
	if wait > gatewayACMECertObtainWaitMax {
		return gatewayACMECertObtainWaitMax
	}
	return wait
}

func (r *RouteReconciler) fleetRoutesYAML(ctx context.Context) []byte {
	return r.fleetRoutesYAMLFromCM(ctx, r.Config.FleetRoutesCMNS, r.Config.FleetRoutesCMName, r.Config.FleetRoutesCMKey)
}

// issueHostname obtains a production Let's Encrypt cert for one hub hostname.
func (r *RouteReconciler) issueHostname(ctx context.Context, hostname string, routesYAML []byte, skipStaging bool) error {
	if hostname == "" {
		return nil
	}
	if r.useACMESolverPath(ctx) {
		if err := r.issueHostnameViaSolver(ctx, hostname, skipStaging); err != nil {
			return err
		}
		if _, err := r.rebuildFleetRoutesCM(ctx); err != nil {
			return err
		}
		return r.reloadGatewayCerts(ctx)
	}
	return r.issueHostnameLegacyGateway(ctx, hostname, routesYAML, skipStaging)
}

// useACMESolverPath is true when solver issuance is configured and gateway-acme exists.
func (r *RouteReconciler) useACMESolverPath(ctx context.Context) bool {
	if !r.Config.ACMESolverEnabled || r.Config.ACMELegacyGatewayScale {
		return false
	}
	var solverSvc corev1.Service
	err := r.Client.Get(ctx, r.acmeSolverServiceKey(), &solverSvc)
	return err == nil
}

// issueHostnameLegacyGateway obtains hub production certs via TLS-ALPN on the main gateway.
func (r *RouteReconciler) issueHostnameLegacyGateway(ctx context.Context, hostname string, routesYAML []byte, skipStaging bool) error {
	depKey := r.gatewayDeploymentKey()
	var dep appsv1.Deployment
	if err := r.Client.Get(ctx, depKey, &dep); err != nil {
		return err
	}

	probeHost := effectiveGatewayProbeHost(r.Config)
	if ok, _ := probeACMECert(probeHost, hostname, false); ok {
		return nil
	}

	// Single-replica hostNetwork hub: fleet routes already expose websecure+letsencrypt routers.
	// Traefik native ACME obtains certs without coordinator-driven gateway rollouts.
	if !gatewayNeedsACMEMode(dep) {
		return r.waitNativeACMECert(ctx, probeHost, hostname, skipStaging)
	}

	scaled := gatewayNeedsACMEMode(dep)
	var orig gatewayACMEProfile
	if scaled {
		orig = gatewayProfile(dep)
		one := int32(1)
		if err := r.applyGatewayACMEMode(ctx, depKey, one); err != nil {
			return err
		}
		if err := r.waitGatewayRollout(ctx, depKey, 5*time.Minute); err != nil {
			_ = r.restoreGatewayProfile(ctx, depKey, orig)
			return err
		}
	}
	cleanup := func() {
		if scaled {
			_ = r.restoreGatewayProfile(ctx, depKey, orig)
			_ = r.waitGatewayRollout(ctx, depKey, 5*time.Minute)
		}
	}
	defer cleanup()

	hosts := []string{hostname}
	runPhase := func(yaml []byte, staging bool) error {
		resolver := traefik.CertResolverProduction
		if staging {
			resolver = traefik.CertResolverPreflight
		}
		phaseYAML, err := traefik.EnsureACMETLSRouters(yaml, hosts, resolver)
		if err != nil {
			return fmt.Errorf("acme routes: %w", err)
		}
		depRoutesChecksum, err := r.gatewayFleetRoutesChecksum(ctx, depKey)
		if err != nil {
			return err
		}
		phaseSum := sha256.Sum256(phaseYAML)
		phaseChecksum := hex.EncodeToString(phaseSum[:])
		currentYAML := r.fleetRoutesYAML(ctx)
		if !bytes.Equal(phaseYAML, currentYAML) {
			if err := r.patchFleetRoutesData(ctx, phaseYAML); err != nil {
				return err
			}
		}
		if depRoutesChecksum != phaseChecksum {
			if err := r.rolloutGatewayChecksum(ctx, depKey, phaseYAML); err != nil {
				return err
			}
			if err := r.waitGatewayRollout(ctx, depKey, 5*time.Minute); err != nil {
				return err
			}
		}
		if err := waitTLSListen(ctx, probeHost, 90*time.Second); err != nil {
			return err
		}
		base := gatewayACMEProductionWait
		if staging {
			base = gatewayACMEPreflightWait
		}
		return waitACMECerts(ctx, probeHost, []string{hostname}, staging, gatewayACMEWaitDuration(base, 1))
	}

	if r.Config.ACMEPreflightEnabled && !skipStaging {
		if err := runPhase(routesYAML, true); err != nil {
			return fmt.Errorf("acme staging preflight failed: %w", err)
		}
	}

	// Drop leftover staging certs so Traefik does not serve them for SNI during production TLS-ALPN.
	if r.Config.ACMEPreflightEnabled {
		if err := clearGatewayPreflightCert(ctx, r.RestConfig, depKey.Namespace, depKey.Name, hostname); err != nil {
			return fmt.Errorf("clear staging acme cert: %w", err)
		}
	}

	if err := runPhase(routesYAML, false); err != nil {
		return fmt.Errorf("acme production issuance: %w", err)
	}
	return nil
}

func (r *RouteReconciler) waitNativeACMECert(ctx context.Context, probeHost, hostname string, skipStaging bool) error {
	if r.Config.ACMEPreflightEnabled && !skipStaging {
		if err := waitTLSListen(ctx, probeHost, 90*time.Second); err != nil {
			return fmt.Errorf("acme staging preflight failed: %w", err)
		}
		if err := waitACMECerts(ctx, probeHost, []string{hostname}, true, gatewayACMEWaitDuration(gatewayACMEPreflightWait, 1)); err != nil {
			return fmt.Errorf("acme staging preflight failed: %w", err)
		}
	}
	if err := waitTLSListen(ctx, probeHost, 90*time.Second); err != nil {
		return fmt.Errorf("acme production issuance: %w", err)
	}
	if err := waitACMECerts(ctx, probeHost, []string{hostname}, false, gatewayACMEWaitDuration(gatewayACMEProductionWait, 1)); err != nil {
		return fmt.Errorf("acme production issuance: %w", err)
	}
	return nil
}

func (r *RouteReconciler) gatewayDeploymentKey() types.NamespacedName {
	ns := r.Config.GatewayDeploymentNS
	if ns == "" {
		ns = r.Config.FleetRoutesCMNS
	}
	name := r.Config.GatewayDeploymentName
	if name == "" {
		name = "gateway"
	}
	return types.NamespacedName{Namespace: ns, Name: name}
}

func (r *RouteReconciler) patchFleetRoutesData(ctx context.Context, routesYAML []byte) error {
	var cm corev1.ConfigMap
	key := types.NamespacedName{Namespace: r.Config.FleetRoutesCMNS, Name: r.Config.FleetRoutesCMName}
	if err := r.Client.Get(ctx, key, &cm); err != nil {
		return err
	}
	patch := client.MergeFrom(cm.DeepCopy())
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[r.Config.FleetRoutesCMKey] = string(routesYAML)
	return r.Client.Patch(ctx, &cm, patch)
}

func (r *RouteReconciler) rolloutGatewayChecksum(ctx context.Context, depKey types.NamespacedName, routesYAML []byte) error {
	var dep appsv1.Deployment
	if err := r.Client.Get(ctx, depKey, &dep); err != nil {
		return err
	}
	sum := sha256.Sum256(routesYAML)
	return r.patchGatewayFleetRoutesChecksum(ctx, &dep, hex.EncodeToString(sum[:]))
}

func (r *RouteReconciler) applyGatewayACMEMode(ctx context.Context, key types.NamespacedName, replicas int32) error {
	var dep appsv1.Deployment
	if err := r.Client.Get(ctx, key, &dep); err != nil {
		return err
	}
	patch := client.MergeFrom(dep.DeepCopy())
	dep.Spec.Replicas = &replicas
	dep.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
	dep.Spec.Template.Spec.HostNetwork = true
	dep.Spec.Template.Spec.DNSPolicy = corev1.DNSClusterFirstWithHostNet
	return r.Patch(ctx, &dep, patch)
}

func (r *RouteReconciler) restoreGatewayProfile(ctx context.Context, key types.NamespacedName, orig gatewayACMEProfile) error {
	var dep appsv1.Deployment
	if err := r.Client.Get(ctx, key, &dep); err != nil {
		return err
	}
	patch := client.MergeFrom(dep.DeepCopy())
	replicas := orig.replicas
	dep.Spec.Replicas = &replicas
	if replicas > 1 {
		dep.Spec.Template.Spec.HostNetwork = false
		dep.Spec.Template.Spec.DNSPolicy = corev1.DNSClusterFirst
	} else {
		dep.Spec.Template.Spec.HostNetwork = orig.hostNetwork
		dep.Spec.Template.Spec.DNSPolicy = orig.dnsPolicy
	}
	dep.Spec.Strategy = appsv1.DeploymentStrategy{Type: orig.strategyType}
	if orig.strategyType == appsv1.RollingUpdateDeploymentStrategyType {
		if orig.rollingUpdate != nil {
			ru := *orig.rollingUpdate
			dep.Spec.Strategy.RollingUpdate = &ru
		} else {
			dep.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{}
		}
	}
	return r.Patch(ctx, &dep, patch)
}

func (r *RouteReconciler) waitGatewayRollout(ctx context.Context, key types.NamespacedName, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var dep appsv1.Deployment
		if err := r.Client.Get(ctx, key, &dep); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		want := int32(1)
		if dep.Spec.Replicas != nil {
			want = *dep.Spec.Replicas
		}
		if !gatewayRolloutComplete(&dep, want) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		if err := r.waitGatewayPodsMatch(ctx, &dep, want); err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("gateway rollout timed out after %s", timeout)
}

func gatewayRolloutComplete(dep *appsv1.Deployment, want int32) bool {
	if dep.Generation > dep.Status.ObservedGeneration {
		return false
	}
	if dep.Status.UpdatedReplicas != want {
		return false
	}
	if dep.Status.AvailableReplicas != want {
		return false
	}
	if dep.Status.ReadyReplicas != want {
		return false
	}
	// Stale pods from a prior ReplicaSet keep status.replicas elevated (e.g. 3+1 during ACME).
	if dep.Status.Replicas != want {
		return false
	}
	return true
}

func (r *RouteReconciler) waitGatewayPodsMatch(ctx context.Context, dep *appsv1.Deployment, want int32) error {
	if dep == nil {
		return fmt.Errorf("deployment is nil")
	}
	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return err
	}
	var podList corev1.PodList
	if err := r.Client.List(ctx, &podList, client.InNamespace(dep.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		// Without pod list RBAC, rely on deployment status counters only.
		if apierrors.IsForbidden(err) {
			return nil
		}
		return err
	}
	running := int32(0)
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		switch pod.Status.Phase {
		case corev1.PodRunning, corev1.PodPending:
			running++
		}
	}
	if running != want {
		return fmt.Errorf("gateway pods running=%d want=%d", running, want)
	}
	return nil
}

func (r *RouteReconciler) gatewayFleetRoutesChecksum(ctx context.Context, depKey types.NamespacedName) (string, error) {
	var dep appsv1.Deployment
	if err := r.Client.Get(ctx, depKey, &dep); err != nil {
		return "", err
	}
	if dep.Spec.Template.Annotations == nil {
		return "", nil
	}
	return dep.Spec.Template.Annotations[gatewayFleetRoutesChecksumKey], nil
}

func (r *RouteReconciler) patchGatewayFleetRoutesChecksum(ctx context.Context, dep *appsv1.Deployment, checksum string) error {
	if dep.Spec.Template.Annotations != nil && dep.Spec.Template.Annotations[gatewayFleetRoutesChecksumKey] == checksum {
		return nil
	}
	patch := client.MergeFrom(dep.DeepCopy())
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}
	dep.Spec.Template.Annotations[gatewayFleetRoutesChecksumKey] = checksum
	return r.Patch(ctx, dep, patch)
}
