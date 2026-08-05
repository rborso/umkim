package reconcile

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/config"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/exec"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/hostnames"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/manifests"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/traefik"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const finalizerName = "edge.umkim.com/route-cleanup"
const annotationFailoverGeneration = "fleet.umkim.com/failover-generation"
const annotationFleetRoutesRebuildKey = "edge.umkim.com/fleet-routes-rebuild-key"

// routeRebuildPredicate reconciles on spec changes, failover annotations, and activeCluster updates.
type routeRebuildPredicate struct {
	predicate.Funcs
}

func (routeRebuildPredicate) Update(e event.UpdateEvent) bool {
	oldRoute, ok1 := e.ObjectOld.(*fleetv1alpha1.FleetAppRoute)
	newRoute, ok2 := e.ObjectNew.(*fleetv1alpha1.FleetAppRoute)
	if !ok1 || !ok2 {
		return false
	}
	if oldRoute.Generation != newRoute.Generation {
		return true
	}
	if oldRoute.Status.ActiveCluster != newRoute.Status.ActiveCluster {
		return true
	}
	oldGen := annotationValue(oldRoute.Annotations, annotationFailoverGeneration)
	newGen := annotationValue(newRoute.Annotations, annotationFailoverGeneration)
	return oldGen != newGen
}

func annotationValue(ann map[string]string, key string) string {
	if ann == nil {
		return ""
	}
	return ann[key]
}

type RouteReconciler struct {
	client.Client
	APIReader client.Reader
	Config    config.Config
	RestConfig rest.Config
	acme      *acmeCoordinator
	spokeACME *spokeACMEQueue
	fleetRoutesMu sync.Mutex
}

func (r *RouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var route fleetv1alpha1.FleetAppRoute
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !route.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&route, finalizerName) {
			if err := r.cleanup(ctx, &route); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&route, finalizerName)
			if err := r.Update(ctx, &route); err != nil {
				return ctrl.Result{}, err
			}
		}
		return r.rebuildFleetRoutesCM(ctx)
	}

	if !controllerutil.ContainsFinalizer(&route, finalizerName) {
		controllerutil.AddFinalizer(&route, finalizerName)
		if err := r.Update(ctx, &route); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	reg, err := clusters.Load(ctx, r.Client, r.Config.ClusterConfigMapNS, r.Config.ClusterConfigMapName)
	if err != nil {
		return r.fail(ctx, &route, err)
	}
	for clusterName, regCluster := range reg.Clusters {
		r.advanceSpokeACMEQueue(ctx, clusterName, regCluster)
	}
	effective, msg := effectiveMode(route.Spec.Mode, route.Spec.Cluster, reg)
	specPatched := false
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest fleetv1alpha1.FleetAppRoute
		if err := r.Get(ctx, req.NamespacedName, &latest); err != nil {
			return err
		}
		base := latest.DeepCopy()
		patchSpec := false
		if latest.Spec.EffectiveMode != effective {
			latest.Spec.EffectiveMode = effective
			patchSpec = true
		}
		if latest.Spec.SiteHost == "" && (effective == fleetv1alpha1.RouteModeSite || effective == fleetv1alpha1.RouteModeDual) {
			siteHost := defaultSiteHost(latest.Spec.HubHost, latest.Spec.Cluster, r.Config.FleetEdgeDomain)
			if latest.Spec.SiteHost != siteHost {
				latest.Spec.SiteHost = siteHost
				patchSpec = true
			}
		}
		if patchSpec {
			if err := r.Patch(ctx, &latest, client.MergeFrom(base)); err != nil {
				return err
			}
			specPatched = true
		}
		route = latest
		return nil
	}); err != nil {
		return ctrl.Result{}, err
	}

	if r.shouldRebuildFleetRoutes(&route, specPatched) {
		if _, err := r.rebuildFleetRoutesCM(ctx); err != nil {
			return r.fail(ctx, &route, err)
		}
		if err := r.markFleetRoutesRebuilt(ctx, &route); err != nil {
			return ctrl.Result{}, err
		}
	}

	if route.Spec.RequireAuth && !r.Config.AutheliaEnabled {
		if msg == "" {
			msg = "ingress auth requested but hub forward-auth disabled"
		} else {
			msg = msg + "; ingress auth requested but hub forward-auth disabled"
		}
	}

	if effective == fleetv1alpha1.RouteModeNone {
		if err := r.patchStatus(ctx, &route, fleetv1alpha1.PhaseApplied, msg); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("route skipped (cluster ingress)", "instance", route.Spec.InstanceID)
		return ctrl.Result{RequeueAfter: time.Duration(r.Config.RequeueSeconds) * time.Second}, nil
	}

	if spokeHosts := manifests.SpokeHostsForRoute(route); len(spokeHosts) > 0 {
		for _, h := range manifests.HostsForRoute(route) {
			if err := hostnames.FindDuplicate(ctx, r.Client, route.Namespace, h, route.Name); err != nil {
				return r.fail(ctx, &route, err)
			}
		}
		if err := manifests.ValidateRouteHosts(route); err != nil {
			return r.fail(ctx, &route, err)
		}
		kc, err := hubKubeconfig()
		if err != nil {
			return r.fail(ctx, &route, err)
		}
		for _, target := range route.PlacementClusters() {
			clusterName := target.Cluster
			if clusterName == "" {
				continue
			}
			regCluster, ok := reg.Get(clusterName)
			if !ok {
				continue
			}
			routeMode := route.Spec.EffectiveMode
			if routeMode == "" {
				routeMode = route.Spec.Mode
			}
			targetOpts := manifests.SpokeIngressOptions{
				AuthEnabled:   r.Config.AutheliaEnabled,
				SpokeACME:     false,
				RedirectHTTPS: regCluster.SpokeACMEEnabled(r.Config.ACMEEnabled),
				// Hub Traefik always upstreams hub/dual catalog hosts to spoke :30081 (web-hub).
				UseHubEntrypoint: regCluster.SpokeACMEEnabled(r.Config.ACMEEnabled) &&
					(routeMode == fleetv1alpha1.RouteModeHub || routeMode == fleetv1alpha1.RouteModeDual),
			}
			if enabled, gateMsg := r.spokeACMEEnabledForRoute(ctx, &route, clusterName, regCluster); enabled {
				targetOpts.SpokeACME = true
				// Queued behind another site hostname: keep existing ManifestWork (do not
				// rewrite TLS ingresses onto web-hub-only).
				if gateMsg == "queued" {
					continue
				}
			}
			yaml, err := manifests.RenderManifestWork(route, spokeHosts, clusterName, target.Service, targetOpts)
			if err != nil {
				return r.fail(ctx, &route, err)
			}
			if err := exec.ApplyYAML(ctx, kc, yaml); err != nil {
				return r.fail(ctx, &route, err)
			}
		}
	}

	if err := r.patchStatus(ctx, &route, fleetv1alpha1.PhaseApplied, msg); err != nil {
		return ctrl.Result{}, err
	}
	logger.Info("route applied", "instance", route.Spec.InstanceID, "mode", effective)
	return ctrl.Result{RequeueAfter: time.Duration(r.Config.RequeueSeconds) * time.Second}, nil
}

func effectiveMode(mode, cluster string, reg clusters.Registry) (string, string) {
	if mode == fleetv1alpha1.RouteModeNone {
		return fleetv1alpha1.RouteModeNone, "cluster ingress only (routing.mode=none)"
	}
	c, ok := reg.Get(cluster)
	if !ok {
		return fleetv1alpha1.RouteModeHub, "cluster not in fleet-edge registry; using hub only"
	}
	if !c.AllowsDirectSiteIngress() && (mode == fleetv1alpha1.RouteModeSite || mode == fleetv1alpha1.RouteModeDual) {
		return fleetv1alpha1.RouteModeHub, "cluster has no direct site ingress; downgraded to hub"
	}
	if mode == "" {
		return fleetv1alpha1.RouteModeHub, ""
	}
	return mode, ""
}

func defaultSiteHost(hubHost, cluster, domain string) string {
	app := hubHost
	for i := 0; i < len(hubHost); i++ {
		if hubHost[i] == '.' {
			app = hubHost[:i]
			break
		}
	}
	return fmt.Sprintf("%s.%s.%s", app, cluster, domain)
}

func (r *RouteReconciler) issuingHubHosts(routes []fleetv1alpha1.FleetAppRoute) []string {
	var hosts []string
	seen := map[string]struct{}{}
	add := func(host string) {
		host = normalizeHostname(host)
		if host == "" {
			return
		}
		if _, ok := seen[host]; ok {
			return
		}
		seen[host] = struct{}{}
		hosts = append(hosts, host)
	}
	for _, route := range routes {
		mode := route.Spec.EffectiveMode
		if mode == "" {
			mode = route.Spec.Mode
		}
		if mode != fleetv1alpha1.RouteModeHub && mode != fleetv1alpha1.RouteModeDual {
			continue
		}
		switch route.Status.TLSPhase {
		case fleetv1alpha1.TLSPhaseQueued, fleetv1alpha1.TLSPhaseIssuing, fleetv1alpha1.TLSPhaseFailed:
			add(route.Spec.HubHost)
		case fleetv1alpha1.TLSPhaseFailedDNS, fleetv1alpha1.TLSPhaseFailedRateLimit:
			continue
		case fleetv1alpha1.TLSPhaseReady, "":
			continue
		default:
			add(route.Spec.HubHost)
		}
	}
	return hosts
}

func (r *RouteReconciler) autheliaPortalHost() string {
	if h := strings.TrimSpace(r.Config.AutheliaPortalHost); h != "" {
		return h
	}
	return "auth." + r.Config.FleetEdgeDomain
}

func (r *RouteReconciler) pendingTLSMiddlewares(routes []fleetv1alpha1.FleetAppRoute, hosts []string) map[string][]string {
	if !r.Config.AutheliaEnabled || r.Config.AutheliaForwardAuthURL == "" {
		return nil
	}
	want := map[string]struct{}{}
	for _, h := range hosts {
		h = normalizeHostname(h)
		if h != "" {
			want[h] = struct{}{}
		}
	}
	if len(want) == 0 {
		return nil
	}
	out := map[string][]string{}
	add := func(host string, requireAuth bool) {
		host = normalizeHostname(host)
		if host == "" {
			return
		}
		if _, ok := want[host]; !ok {
			return
		}
		if !requireAuth {
			return
		}
		out[host] = []string{"fleet-edge-forward-headers-https", "fleet-edge-authelia"}
	}
	for _, route := range routes {
		mode := route.Spec.EffectiveMode
		if mode == "" {
			mode = route.Spec.Mode
		}
		if mode != fleetv1alpha1.RouteModeHub && mode != fleetv1alpha1.RouteModeDual {
			continue
		}
		add(route.Spec.HubHost, route.Spec.RequireAuth)
	}
	portal := normalizeHostname(r.autheliaPortalHost())
	if _, ok := want[portal]; ok {
		out[portal] = []string{"fleet-edge-forward-headers-https"}
	}
	return out
}

func fleetRoutesRebuildKey(route *fleetv1alpha1.FleetAppRoute) string {
	failoverGen := annotationValue(route.Annotations, annotationFailoverGeneration)
	active := route.Status.ActiveCluster
	return fmt.Sprintf("g%d:f%s:a%s", route.Generation, failoverGen, active)
}

func (r *RouteReconciler) shouldRebuildFleetRoutes(route *fleetv1alpha1.FleetAppRoute, specPatched bool) bool {
	if specPatched {
		return true
	}
	want := fleetRoutesRebuildKey(route)
	if route.Annotations == nil {
		return true
	}
	return route.Annotations[annotationFleetRoutesRebuildKey] != want
}

func (r *RouteReconciler) markFleetRoutesRebuilt(ctx context.Context, route *fleetv1alpha1.FleetAppRoute) error {
	want := fleetRoutesRebuildKey(route)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest fleetv1alpha1.FleetAppRoute
		if err := r.Get(ctx, types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, &latest); err != nil {
			return err
		}
		if latest.Annotations != nil && latest.Annotations[annotationFleetRoutesRebuildKey] == want {
			return nil
		}
		base := latest.DeepCopy()
		if latest.Annotations == nil {
			latest.Annotations = map[string]string{}
		}
		latest.Annotations[annotationFleetRoutesRebuildKey] = want
		return r.Patch(ctx, &latest, client.MergeFrom(base))
	})
}

func (r *RouteReconciler) rebuildFleetRoutesCM(ctx context.Context) (ctrl.Result, error) {
	r.fleetRoutesMu.Lock()
	defer r.fleetRoutesMu.Unlock()

	var list fleetv1alpha1.FleetAppRouteList
	if err := r.List(ctx, &list); err != nil {
		return ctrl.Result{}, err
	}
	reg, err := clusters.Load(ctx, r.Client, r.Config.ClusterConfigMapNS, r.Config.ClusterConfigMapName)
	if err != nil {
		return ctrl.Result{}, err
	}
	for clusterName, regCluster := range reg.Clusters {
		r.advanceSpokeACMEQueue(ctx, clusterName, regCluster)
	}
	routes := append([]fleetv1alpha1.FleetAppRoute(nil), list.Items...)
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Name < routes[j].Name
	})
	data, err := traefik.Build(routes, reg, r.Config.ACMEEnabled, traefik.BuildOptions{
		AutheliaEnabled:        r.Config.AutheliaEnabled,
		AutheliaForwardAuthURL: r.Config.AutheliaForwardAuthURL,
		AutheliaPortalHost:     r.autheliaPortalHost(),
		AutheliaServiceURL:     r.Config.AutheliaServiceURL,
		CertsReady:             r.buildCertsReadyMap(ctx, routes),
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if r.Config.ACMEEnabled {
		if issuing := r.issuingHubHosts(routes); len(issuing) > 0 {
			data, err = traefik.EnsureACMETLSRoutersWithMiddlewares(
				data, issuing, traefik.CertResolverProduction, r.pendingTLSMiddlewares(routes, issuing))
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	prevRoutesYAML := r.fleetRoutesYAMLFromCM(ctx, r.Config.FleetRoutesCMNS, r.Config.FleetRoutesCMName, r.Config.FleetRoutesCMKey)

	// Avoid fighting hub ACME issuance (legacy hostNetwork): coordinator patches fleet routes
	// during TLS-ALPN; periodic route reconciles must not overwrite mid-issuance.
	if r.acme != nil && r.acme.Issuing() {
		return ctrl.Result{}, nil
	}
	if bytes.Equal(prevRoutesYAML, data) {
		return ctrl.Result{}, nil
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Config.FleetRoutesCMName,
			Namespace: r.Config.FleetRoutesCMNS,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data[r.Config.FleetRoutesCMKey] = string(data)
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.rolloutGatewayForFleetRoutes(ctx, prevRoutesYAML, data); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *RouteReconciler) cleanup(ctx context.Context, route *fleetv1alpha1.FleetAppRoute) error {
	kc, err := hubKubeconfig()
	if err != nil {
		return err
	}
	workName := manifests.WorkNameForInstance(route.Spec.InstanceID)
	for _, clusterName := range route.ClusterNamesForCleanup() {
		if err := exec.DeleteManifestWork(ctx, kc, clusterName, workName); err != nil {
			return err
		}
	}
	return nil
}

func (r *RouteReconciler) fail(ctx context.Context, route *fleetv1alpha1.FleetAppRoute, err error) (ctrl.Result, error) {
	_ = r.patchStatus(ctx, route, fleetv1alpha1.PhaseDegraded, err.Error())
	return ctrl.Result{RequeueAfter: time.Duration(r.Config.RequeueSeconds) * time.Second}, err
}

func (r *RouteReconciler) patchStatus(ctx context.Context, route *fleetv1alpha1.FleetAppRoute, phase, msg string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var latest fleetv1alpha1.FleetAppRoute
		if err := r.Get(ctx, types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, &latest); err != nil {
			return err
		}
		base := latest.DeepCopy()
		latest.Status.Phase = phase
		latest.Status.Message = msg
		return r.Status().Patch(ctx, &latest, client.MergeFrom(base))
	})
}

func hubKubeconfig() (string, error) {
	if p := os.Getenv("KUBECONFIG"); p != "" {
		return p, nil
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return "", nil
	}
	return "", fmt.Errorf("KUBECONFIG is not set and not running in-cluster")
}

func (r *RouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.acme = newACMECoordinator(r.Config, r)
	if err := mgr.Add(r.acme); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&fleetv1alpha1.FleetAppRoute{}).
		WithEventFilter(predicate.Or(
			predicate.GenerationChangedPredicate{},
			routeRebuildPredicate{},
		)).
		Complete(r)
}

// Ensure ConfigMap exists before first route.
func (r *RouteReconciler) ensureFleetCM(ctx context.Context) error {
	var cm corev1.ConfigMap
	err := r.Get(ctx, types.NamespacedName{Namespace: r.Config.FleetRoutesCMNS, Name: r.Config.FleetRoutesCMName}, &cm)
	if apierrors.IsNotFound(err) {
		cm = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.Config.FleetRoutesCMName,
				Namespace: r.Config.FleetRoutesCMNS,
			},
			Data: map[string]string{r.Config.FleetRoutesCMKey: "{}\n"},
		}
		return r.Create(ctx, &cm)
	}
	return err
}
