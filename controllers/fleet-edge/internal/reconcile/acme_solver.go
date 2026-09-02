package reconcile

import (
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

const acmeSolverRoutesChecksumKey = "checksum/fleet-traefik-acme-routes"

type wanHandoffState struct {
	gatewayExternalIPs []string
}

func (r *RouteReconciler) acmeSolverDeploymentKey() types.NamespacedName {
	return types.NamespacedName{
		Namespace: r.Config.GatewayDeploymentNS,
		Name:      r.Config.ACMESolverDeploymentName,
	}
}

func (r *RouteReconciler) acmeSolverServiceKey() types.NamespacedName {
	return types.NamespacedName{
		Namespace: r.Config.GatewayDeploymentNS,
		Name:      r.Config.ACMESolverServiceName,
	}
}

func (r *RouteReconciler) handoffWANToSolver(ctx context.Context) (*wanHandoffState, error) {
	gwSvcKey := types.NamespacedName{Namespace: r.Config.GatewayDeploymentNS, Name: r.Config.GatewayDeploymentName}
	var gwSvc corev1.Service
	if err := r.Client.Get(ctx, gwSvcKey, &gwSvc); err != nil {
		return nil, err
	}
	state := &wanHandoffState{gatewayExternalIPs: append([]string(nil), gwSvc.Spec.ExternalIPs...)}
	if len(state.gatewayExternalIPs) == 0 {
		if ip := r.Config.GatewayWANIP; ip != "" {
			state.gatewayExternalIPs = []string{ip}
		} else if ip := r.Config.GatewayProbeHost; ip != "" && ip != "127.0.0.1" {
			state.gatewayExternalIPs = []string{ip}
		}
	}

	gwPatch := client.MergeFrom(gwSvc.DeepCopy())
	gwSvc.Spec.ExternalIPs = nil
	if err := r.Client.Patch(ctx, &gwSvc, gwPatch); err != nil {
		return nil, fmt.Errorf("clear gateway externalIPs: %w", err)
	}

	var solverSvc corev1.Service
	solverKey := r.acmeSolverServiceKey()
	if err := r.Client.Get(ctx, solverKey, &solverSvc); err != nil {
		_ = r.restoreGatewayExternalIPs(ctx, state)
		return nil, err
	}
	solverPatch := client.MergeFrom(solverSvc.DeepCopy())
	solverSvc.Spec.ExternalIPs = append([]string(nil), state.gatewayExternalIPs...)
	if err := r.Client.Patch(ctx, &solverSvc, solverPatch); err != nil {
		_ = r.restoreGatewayExternalIPs(ctx, state)
		return nil, fmt.Errorf("set solver externalIPs: %w", err)
	}

	one := int32(1)
	depKey := r.acmeSolverDeploymentKey()
	var dep appsv1.Deployment
	if err := r.Client.Get(ctx, depKey, &dep); err != nil {
		_ = r.handoffWANToGateway(ctx, state)
		return nil, err
	}
	depPatch := client.MergeFrom(dep.DeepCopy())
	dep.Spec.Replicas = &one
	if err := r.Client.Patch(ctx, &dep, depPatch); err != nil {
		_ = r.handoffWANToGateway(ctx, state)
		return nil, err
	}
	if err := r.waitGatewayRollout(ctx, depKey, 5*time.Minute); err != nil {
		_ = r.handoffWANToGateway(ctx, state)
		return nil, err
	}
	return state, nil
}

func (r *RouteReconciler) handoffWANToGateway(ctx context.Context, state *wanHandoffState) error {
	if state == nil {
		return nil
	}
	zero := int32(0)
	depKey := r.acmeSolverDeploymentKey()
	var dep appsv1.Deployment
	if err := r.Client.Get(ctx, depKey, &dep); err == nil {
		depPatch := client.MergeFrom(dep.DeepCopy())
		dep.Spec.Replicas = &zero
		_ = r.Client.Patch(ctx, &dep, depPatch)
		_ = r.waitGatewayRollout(ctx, depKey, 3*time.Minute)
	}

	solverKey := r.acmeSolverServiceKey()
	var solverSvc corev1.Service
	if err := r.Client.Get(ctx, solverKey, &solverSvc); err == nil {
		sp := client.MergeFrom(solverSvc.DeepCopy())
		solverSvc.Spec.ExternalIPs = nil
		_ = r.Client.Patch(ctx, &solverSvc, sp)
	}
	return r.restoreGatewayExternalIPs(ctx, state)
}

func (r *RouteReconciler) restoreGatewayExternalIPs(ctx context.Context, state *wanHandoffState) error {
	if state == nil || len(state.gatewayExternalIPs) == 0 {
		return nil
	}
	gwSvcKey := types.NamespacedName{Namespace: r.Config.GatewayDeploymentNS, Name: r.Config.GatewayDeploymentName}
	var gwSvc corev1.Service
	if err := r.Client.Get(ctx, gwSvcKey, &gwSvc); err != nil {
		return err
	}
	patch := client.MergeFrom(gwSvc.DeepCopy())
	gwSvc.Spec.ExternalIPs = append([]string(nil), state.gatewayExternalIPs...)
	return r.Client.Patch(ctx, &gwSvc, patch)
}

func (r *RouteReconciler) patchACMESolverRoutesData(ctx context.Context, routesYAML []byte) error {
	var cm corev1.ConfigMap
	key := types.NamespacedName{Namespace: r.Config.FleetRoutesCMNS, Name: r.Config.ACMESolverRoutesCMName}
	if err := r.Client.Get(ctx, key, &cm); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		cm = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.Config.ACMESolverRoutesCMName,
				Namespace: r.Config.FleetRoutesCMNS,
			},
		}
		if err := r.Client.Create(ctx, &cm); err != nil {
			return err
		}
	}
	patch := client.MergeFrom(cm.DeepCopy())
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[r.Config.ACMESolverRoutesCMKey] = string(routesYAML)
	return r.Client.Patch(ctx, &cm, patch)
}

func (r *RouteReconciler) rolloutACMESolverChecksum(ctx context.Context, routesYAML []byte) error {
	depKey := r.acmeSolverDeploymentKey()
	var dep appsv1.Deployment
	if err := r.Client.Get(ctx, depKey, &dep); err != nil {
		return err
	}
	sum := sha256.Sum256(routesYAML)
	patch := client.MergeFrom(dep.DeepCopy())
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}
	dep.Spec.Template.Annotations[acmeSolverRoutesChecksumKey] = hex.EncodeToString(sum[:])
	return r.Patch(ctx, &dep, patch)
}

func (r *RouteReconciler) issueHostnameViaSolver(ctx context.Context, hostname string, skipStaging bool) error {
	state, err := r.handoffWANToSolver(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = r.handoffWANToGateway(ctx, state) }()

	probeHost := r.Config.GatewayProbeHost
	if probeHost == "" {
		probeHost = "127.0.0.1"
	}
	runPhase := func(resolver string, staging bool) error {
		yaml, err := traefik.BuildACMESolverRoutes([]string{hostname}, resolver)
		if err != nil {
			return err
		}
		if err := r.patchACMESolverRoutesData(ctx, yaml); err != nil {
			return err
		}
		if err := r.rolloutACMESolverChecksum(ctx, yaml); err != nil {
			return err
		}
		if err := r.waitGatewayRollout(ctx, r.acmeSolverDeploymentKey(), 5*time.Minute); err != nil {
			return err
		}
		base := gatewayACMEProductionWait
		if staging {
			base = gatewayACMEPreflightWait
		}
		return waitACMECerts(ctx, probeHost, []string{hostname}, staging, gatewayACMEWaitDuration(base, 1))
	}

	if r.Config.ACMEPreflightEnabled && !skipStaging {
		if err := runPhase(traefik.CertResolverPreflight, true); err != nil {
			return fmt.Errorf("acme staging preflight failed: %w", err)
		}
	}
	if err := runPhase(traefik.CertResolverProduction, false); err != nil {
		return fmt.Errorf("acme production issuance: %w", err)
	}
	return nil
}

func (r *RouteReconciler) reloadGatewayCerts(ctx context.Context) error {
	depKey := r.gatewayDeploymentKey()
	routesYAML := r.fleetRoutesYAML(ctx)
	if len(routesYAML) == 0 {
		routesYAML = []byte("{}\n")
	}
	return r.rolloutGatewayChecksum(ctx, depKey, routesYAML)
}
