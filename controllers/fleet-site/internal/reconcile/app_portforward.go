package reconcile

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// tcpForwardLabel marks the hub-side port-forwarder Deployments UMKIM owns.
	tcpForwardLabel = "fleet.umkim.com/tcp-forward"
	appLabel        = "fleet.umkim.com/app"
	// tcpForwardImage is a tiny socat image: listen on the hub, dial the spoke.
	tcpForwardImage = "docker.io/alpine/socat:1.8.0.0"
)

// ensureTCPForwarders materializes a hub-side L4 port forward for each `tcp`
// route on the app and prunes forwarders whose route is gone.
//
// A tcp route has no hostname, so it cannot ride the HTTP edge: UMKIM runs a
// port-forwarder on the hub (host network) that listens on the route's
// forwardPort and dials servicePort on the target cluster's node over the
// WireGuard overlay. The app's Service must be published as a NodePort for that
// to resolve (see the chart values).
func (r *AppReconciler) ensureTCPForwarders(ctx context.Context, app *fleetv1alpha1.FleetApp, release string) error {
	logger := log.FromContext(ctx)
	desired := map[string]struct{}{}
	for i, rt := range app.Spec.EffectiveRoutes() {
		if !rt.IsTCP() {
			continue
		}
		name := routeObjectName(app.Name, i, rt)
		if err := r.ensureTCPForwarder(ctx, app, release, name, rt); err != nil {
			return err
		}
		desired[tcpForwardName(name)] = struct{}{}
	}
	var existing appsv1.DeploymentList
	if err := r.List(ctx, &existing,
		client.InNamespace(app.Namespace),
		client.MatchingLabels{tcpForwardLabel: "true", appLabel: app.Name}); err != nil {
		return err
	}
	for i := range existing.Items {
		dep := &existing.Items[i]
		if _, ok := desired[dep.Name]; ok {
			continue
		}
		if err := r.Delete(ctx, dep); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		logger.Info("removed stale tcp port forwarder", "deployment", dep.Name)
	}
	return nil
}

func (r *AppReconciler) ensureTCPForwarder(ctx context.Context, app *fleetv1alpha1.FleetApp, release, name string, rt fleetv1alpha1.FleetAppRouteIntent) error {
	if rt.ForwardPort <= 0 {
		return fmt.Errorf("route %q: forwardPort is required for protocol %s", name, fleetv1alpha1.AppRouteProtocolTCP)
	}
	if rt.ServicePort <= 0 {
		return fmt.Errorf("route %q: servicePort is required for protocol %s (the target node port)", name, fleetv1alpha1.AppRouteProtocolTCP)
	}
	targetIP, err := r.clusterOverlayIP(ctx, app.Spec.ClusterName)
	if err != nil {
		return err
	}
	labels := map[string]string{tcpForwardLabel: "true", appLabel: app.Name}
	replicas := int32(1)
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: tcpForwardName(name), Namespace: app.Namespace}}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		dep.Labels = labels
		dep.Spec = appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					// Host network: the forwarder must listen on the hub itself.
					HostNetwork: true,
					Containers: []corev1.Container{{
						Name:  "forward",
						Image: tcpForwardImage,
						Args: []string{
							fmt.Sprintf("TCP-LISTEN:%d,fork,reuseaddr", rt.ForwardPort),
							fmt.Sprintf("TCP:%s:%d", targetIP, rt.ServicePort),
						},
						Ports: []corev1.ContainerPort{{
							Name:          "forward",
							ContainerPort: rt.ForwardPort,
							Protocol:      corev1.ProtocolTCP,
						}},
					}},
				},
			},
		}
		return ctrl.SetControllerReference(app, dep, r.Scheme)
	})
	return err
}

func tcpForwardName(routeName string) string {
	return "fleet-app-pf-" + routeName
}

// clusterOverlayIP returns the target cluster's WireGuard address (the hub's
// route to that cluster's nodes).
func (r *AppReconciler) clusterOverlayIP(ctx context.Context, cluster string) (string, error) {
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: cluster}, &fcj); err != nil {
		return "", fmt.Errorf("cluster %q: %w", cluster, err)
	}
	ip := strings.TrimSpace(fcj.Status.WireguardAddress)
	ip = strings.TrimPrefix(ip, "[")
	if i := strings.Index(ip, "/"); i >= 0 {
		ip = ip[:i]
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return "", fmt.Errorf("cluster %q has no wireguard address yet", cluster)
	}
	return ip, nil
}
