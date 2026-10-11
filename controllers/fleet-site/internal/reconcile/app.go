package reconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	fleetedge "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/appchart"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingressauth"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/llm"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type AppReconciler struct {
	*Reconciler
}

func (r *AppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var app fleetv1alpha1.FleetApp
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !app.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &app)
	}

	if !controllerutil.ContainsFinalizer(&app, fleetv1alpha1.FleetAppFinalizer) {
		controllerutil.AddFinalizer(&app, fleetv1alpha1.FleetAppFinalizer)
		if err := r.Update(ctx, &app); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	chart := appchart.ExpandChart(app.Spec.Chart, r.Cfg.DefaultChartPrefix())
	if err := appchart.ChartAllowed(chart, r.Cfg.ChartOCIPrefixes()); err != nil {
		return r.failApp(ctx, &app, err.Error())
	}
	release := app.Spec.EffectiveReleaseName(app.Name)
	ns := app.Spec.EffectiveNamespace()
	if !appchart.ValidDNS1123(app.Spec.ClusterName) || !appchart.ValidDNS1123(release) || !appchart.ValidDNS1123(ns) {
		return r.failApp(ctx, &app, "clusterName, releaseName, and namespace must be DNS-1123 labels")
	}
	if !appchart.ValidVersion(app.Spec.Version) {
		return r.failApp(ctx, &app, "invalid chart version")
	}
	mode := app.Spec.EffectiveRouteMode()
	if !appchart.ValidRouteMode(mode) {
		return r.failApp(ctx, &app, "route.mode must be none, hub, or site")
	}

	// Register/refresh the app's Authelia OIDC client on its cluster.
	if err := r.ensureAppOIDCClient(ctx, &app); err != nil {
		return ctrl.Result{}, err
	}

	hash := appSpecHash(app.Spec)
	if app.Status.Phase == fleetv1alpha1.AppPhaseInstalling {
		return r.pollInstall(ctx, &app, chart, release, ns, hash)
	}
	specUnchanged := app.Status.SpecHash == hash && app.Status.ObservedGeneration == app.Generation
	if app.Status.Phase == fleetv1alpha1.AppPhaseReady && specUnchanged {
		if err := r.ensureRoute(ctx, &app, release, ns, mode); err != nil {
			logger.Error(err, "ensure FleetAppRoute")
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}
	if app.Status.Phase == fleetv1alpha1.AppPhaseFailed && specUnchanged {
		return ctrl.Result{}, nil
	}

	if err := r.ensureValuesCM(ctx, &app); err != nil {
		return r.failApp(ctx, &app, err.Error())
	}
	imageMapCM := ""
	if fc := r.libraryChart(ctx, &app); fc != nil && len(fc.Status.ImagesCopied) > 0 {
		if err := r.ensureImageMapCM(ctx, &app, fc); err != nil {
			return r.failApp(ctx, &app, err.Error())
		}
		if !chartImageValuesOverride(fc) {
			imageMapCM = appImageMapCMName(app.Name)
		}
	}
	jobName := appJobName("fa-i-", app.Name)
	job := jobs.HelmAppJob(jobs.HelmAppJobOpts{
		Namespace:         app.Namespace,
		JobName:           jobName,
		Cluster:           app.Spec.ClusterName,
		Chart:             chart,
		Version:           strings.TrimSpace(app.Spec.Version),
		Release:           release,
		TargetNamespace:   ns,
		Action:            "install",
		RunnerImage:       r.Cfg.AnsibleRunnerImage,
		ValuesConfigMap:   appValuesCMName(app.Name),
		ImageMapConfigMap: imageMapCM,
		RegistryHost:      appchart.NeedsRegistryLogin(chart),
		HubSecretsName:    jobs.HubAnsibleSecretsName,
	})
	if err := controllerutil.SetControllerReference(&app, job, r.Scheme); err != nil {
		return r.failApp(ctx, &app, err.Error())
	}
	if err := r.ensureProviderJob(ctx, job); err != nil {
		if isJobTerminating(err) {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return r.failApp(ctx, &app, err.Error())
	}
	app.Status.Phase = fleetv1alpha1.AppPhaseInstalling
	app.Status.Message = "Helm install running on " + app.Spec.ClusterName
	app.Status.LastJobName = jobName
	app.Status.ClusterName = app.Spec.ClusterName
	app.Status.ReleaseName = release
	app.Status.Namespace = ns
	app.Status.Chart = chart
	app.Status.SpecHash = hash
	app.Status.ObservedGeneration = app.Generation
	if err := r.Status().Update(ctx, &app); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *AppReconciler) pollInstall(ctx context.Context, app *fleetv1alpha1.FleetApp, chart, release, ns, hash string) (ctrl.Result, error) {
	done, ok, msg := r.jobPollState(ctx, app.Namespace, app.Status.LastJobName, false)
	if !done {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if !ok {
		return r.failApp(ctx, app, "helm install failed: "+msg)
	}
	app.Status.Phase = fleetv1alpha1.AppPhaseReady
	app.Status.Message = "Helm release " + release + " installed on " + app.Spec.ClusterName
	app.Status.SpecHash = hash
	app.Status.ObservedGeneration = app.Generation
	app.Status.Chart = chart
	app.Status.ReleaseName = release
	app.Status.Namespace = ns
	app.Status.ClusterName = app.Spec.ClusterName
	if err := r.ensureRoute(ctx, app, release, ns, app.Spec.EffectiveRouteMode()); err != nil {
		app.Status.Message += "; route: " + err.Error()
	}
	if err := r.Status().Update(ctx, app); err != nil {
		return ctrl.Result{}, err
	}
	if llm.IsLlamaChart(app.Spec.Chart) && app.RegistersLLMBackend() {
		if err := r.requestFleetLLMReconcile(ctx); err != nil {
			logger := log.FromContext(ctx)
			logger.Error(err, "request FleetLLM reconcile after llama-cpp app ready")
		}
	}
	return ctrl.Result{}, nil
}

func (r *AppReconciler) reconcileDelete(ctx context.Context, app *fleetv1alpha1.FleetApp) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(app, fleetv1alpha1.FleetAppFinalizer) {
		return ctrl.Result{}, nil
	}
	_ = r.deleteRoute(ctx, app)
	_ = r.removeAppOIDCClient(ctx, app)
	release := app.Spec.EffectiveReleaseName(app.Name)
	ns := app.Spec.EffectiveNamespace()
	jobName := appJobName("fa-u-", app.Name)
	if app.Status.Phase != fleetv1alpha1.AppPhaseDeleting {
		job := jobs.HelmAppJob(jobs.HelmAppJobOpts{
			Namespace:       app.Namespace,
			JobName:         jobName,
			Cluster:         app.Spec.ClusterName,
			Chart:           app.Status.Chart,
			Release:         release,
			TargetNamespace: ns,
			Action:          "uninstall",
			RunnerImage:     r.Cfg.AnsibleRunnerImage,
			HubSecretsName:  jobs.HubAnsibleSecretsName,
		})
		if err := r.ensureProviderJob(ctx, job); err != nil {
			if isJobTerminating(err) {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			log.FromContext(ctx).Error(err, "helm uninstall job; continuing delete")
		} else {
			app.Status.Phase = fleetv1alpha1.AppPhaseDeleting
			app.Status.Message = "Uninstalling Helm release"
			app.Status.LastJobName = jobName
			_ = r.Status().Update(ctx, app)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
	} else {
		done, ok, msg := r.jobPollState(ctx, app.Namespace, jobName, true)
		if !done {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		if !ok {
			app.Status.Message = "uninstall failed: " + msg
			_ = r.Status().Update(ctx, app)
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
	}
	_ = r.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: appValuesCMName(app.Name), Namespace: app.Namespace}})
	llamaDeleted := llm.IsLlamaChart(app.Spec.Chart)
	controllerutil.RemoveFinalizer(app, fleetv1alpha1.FleetAppFinalizer)
	if err := r.Update(ctx, app); err != nil {
		return ctrl.Result{}, err
	}
	if llamaDeleted {
		if err := r.requestFleetLLMReconcile(ctx); err != nil {
			log.FromContext(ctx).Error(err, "request FleetLLM reconcile after llama-cpp app delete")
		}
	}
	return ctrl.Result{}, nil
}

func (r *AppReconciler) ensureValuesCM(ctx context.Context, app *fleetv1alpha1.FleetApp) error {
	data := strings.TrimSpace(app.Spec.ValuesYAML)
	if fc := r.libraryChart(ctx, app); fc != nil && strings.TrimSpace(fc.Status.ImageValues) != "" {
		merged, err := appchart.MergeValuesYAML(fc.Status.ImageValues, data)
		if err != nil {
			return err
		}
		data = merged
	}
	data, err := r.substitutePlaceholders(ctx, app, data)
	if err != nil {
		return err
	}
	if data == "" {
		data = "{}\n"
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: appValuesCMName(app.Name), Namespace: app.Namespace},
		Data:       map[string]string{"values.yaml": data},
	}
	if err := controllerutil.SetControllerReference(app, cm, r.Scheme); err != nil {
		return err
	}
	var existing corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, &existing); apierrors.IsNotFound(err) {
		return r.Create(ctx, cm)
	} else if err != nil {
		return err
	}
	existing.Data = cm.Data
	return r.Update(ctx, &existing)
}

// substitutePlaceholders resolves UMKIM-managed values in the app values:
//   ${APP_HOST}                  public host of the app's route
//   ${FORWARD_AUTH}              "true" when any route requires Authelia
//   ${FORWARD_AUTH_MIDDLEWARE}   the Authelia forward-auth middleware name
//   ${OIDC_ISSUER_URL}           cluster Authelia OIDC issuer
//   ${OIDC_CLIENT_ID}            app OIDC client id
//   ${OIDC_CLIENT_SECRET}        generated app OIDC client secret
//
// This lets a chart opt into UMKIM's route/OIDC wiring without the operator
// wiring hosts, middleware names, or secrets by hand.
func (r *AppReconciler) substitutePlaceholders(ctx context.Context, app *fleetv1alpha1.FleetApp, data string) (string, error) {
	if !strings.Contains(data, "${") {
		return data, nil
	}
	host := ""
	forwardAuth := "false"
	for _, rt := range app.Spec.EffectiveRoutes() {
		h := strings.TrimSpace(rt.SiteHost)
		if h == "" {
			h = strings.TrimSpace(rt.HubHost)
		}
		if host == "" && h != "" {
			host = h
		}
		if rt.RequireAuth {
			forwardAuth = "true"
		}
	}
	data = strings.ReplaceAll(data, "${APP_HOST}", host)
	data = strings.ReplaceAll(data, "${FORWARD_AUTH}", forwardAuth)
	data = strings.ReplaceAll(data, "${FORWARD_AUTH_MIDDLEWARE}", "fleet-ingress-authelia@file")

	if !strings.Contains(data, "${OIDC_") {
		return data, nil
	}
	clientID := strings.TrimSpace(app.Spec.OIDCClientID)
	if clientID == "" {
		clientID = app.Name
	}
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: app.Spec.ClusterName}, &fcj); err != nil {
		return data, nil
	}
	issuer := ""
	if dns, err := ingressauth.ResolveClusterDNS(ctx, r.Client, r.Cfg, app.Spec.ClusterName, &fcj); err == nil && dns.PortalHost != "" {
		issuer = "https://" + dns.PortalHost
	}
	secret := ""
	var sec corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: ingressauth.ClusterSecretName(app.Spec.ClusterName)}, &sec); err == nil {
		m := map[string]string{}
		if raw := sec.Data["oidcClientSecrets"]; len(raw) > 0 {
			_ = json.Unmarshal(raw, &m)
		}
		secret = m[clientID]
	}
	data = strings.ReplaceAll(data, "${OIDC_ISSUER_URL}", issuer)
	data = strings.ReplaceAll(data, "${OIDC_CLIENT_ID}", clientID)
	data = strings.ReplaceAll(data, "${OIDC_CLIENT_SECRET}", secret)
	return data, nil
}

func (r *AppReconciler) libraryChart(ctx context.Context, app *fleetv1alpha1.FleetApp) *fleetv1alpha1.FleetChart {
	name := appchart.LibraryName(app.Spec.Chart, r.Cfg.DefaultChartPrefix())
	if name == "" {
		return nil
	}
	var fc fleetv1alpha1.FleetChart
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: app.Namespace}, &fc); err != nil {
		return nil
	}
	return &fc
}

func chartImageValuesOverride(fc *fleetv1alpha1.FleetChart) bool {
	v := strings.TrimSpace(fc.Status.ImageValues)
	return strings.Contains(v, "\nimage:") || strings.HasPrefix(v, "image:")
}

func expandImageMapEntries(from, to string) map[string]string {
	out := map[string]string{}
	if from == "" || to == "" {
		return out
	}
	add := func(ref string) {
		if ref != "" {
			out[ref] = to
		}
	}
	add(from)
	if strings.HasPrefix(from, "docker.io/library/") {
		add(strings.TrimPrefix(from, "docker.io/library/"))
		add(strings.TrimPrefix(from, "docker.io/"))
	} else if strings.HasPrefix(from, "docker.io/") {
		add(strings.TrimPrefix(from, "docker.io/"))
	}
	return out
}

func (r *AppReconciler) ensureImageMapCM(ctx context.Context, app *fleetv1alpha1.FleetApp, fc *fleetv1alpha1.FleetChart) error {
	m := map[string]string{}
	for _, img := range fc.Status.ImagesCopied {
		for ref, dst := range expandImageMapEntries(img.From, img.To) {
			m[ref] = dst
		}
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: appImageMapCMName(app.Name), Namespace: app.Namespace},
		Data:       map[string]string{"images.json": string(raw)},
	}
	if err := controllerutil.SetControllerReference(app, cm, r.Scheme); err != nil {
		return err
	}
	var existing corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, &existing); apierrors.IsNotFound(err) {
		return r.Create(ctx, cm)
	} else if err != nil {
		return err
	}
	existing.Data = cm.Data
	return r.Update(ctx, &existing)
}

func (r *AppReconciler) ensureRoute(ctx context.Context, app *fleetv1alpha1.FleetApp, release, ns, mode string) error {
	return r.ensureRoutes(ctx, app, release)
}

// ensureRoutes materializes one FleetAppRoute per spec.routes entry (or the single
// legacy spec.route) and removes routes no longer declared.
func (r *AppReconciler) ensureRoutes(ctx context.Context, app *fleetv1alpha1.FleetApp, release string) error {
	routeNS := r.Cfg.EdgeRoutesNamespace
	if routeNS == "" {
		routeNS = "fleet-system"
	}
	desired := map[string]struct{}{}
	names := []string{}
	for i, rt := range app.Spec.EffectiveRoutes() {
		if rt.EffectiveMode() == fleetv1alpha1.AppRouteModeNone {
			continue
		}
		name := routeObjectName(app.Name, i, rt)
		if err := r.ensureOneRoute(ctx, app, release, routeNS, name, rt); err != nil {
			return err
		}
		desired[name] = struct{}{}
		names = append(names, name)
	}
	var existing fleetedge.FleetAppRouteList
	if err := r.List(ctx, &existing, client.InNamespace(routeNS), client.MatchingLabels{"fleet.umkim.com/app": app.Name}); err == nil {
		for i := range existing.Items {
			if _, ok := desired[existing.Items[i].Name]; !ok {
				_ = r.Delete(ctx, &existing.Items[i])
			}
		}
	}
	sort.Strings(names)
	app.Status.RouteNames = names
	app.Status.RouteName = ""
	if len(names) > 0 {
		app.Status.RouteName = names[0]
	}
	return nil
}

// routeObjectName returns the FleetAppRoute name for a route: the app name for the
// first route (back-compat) and <app>-<key> for the rest.
func routeObjectName(appName string, index int, rt fleetv1alpha1.FleetAppRouteIntent) string {
	if index == 0 {
		return appName
	}
	key := sanitizeRouteKey(rt.RouteKey(rt.ServiceName))
	if key == "" {
		key = fmt.Sprintf("r%d", index)
	}
	return appName + "-" + key
}

func sanitizeRouteKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, ch := range s {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9', ch == '-':
			b.WriteRune(ch)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 40 {
		out = out[:40]
	}
	return strings.Trim(out, "-")
}

func (r *AppReconciler) ensureOneRoute(ctx context.Context, app *fleetv1alpha1.FleetApp, release, routeNS, name string, rt fleetv1alpha1.FleetAppRouteIntent) error {
	mode := rt.EffectiveMode()
	hubHost := strings.TrimSpace(rt.HubHost)
	if hubHost == "" && mode == fleetv1alpha1.AppRouteModeHub {
		hubHost = defaultRouteHostname(release, r.Cfg.HubEdgeDomain)
	}
	if hubHost == "" && mode == fleetv1alpha1.AppRouteModeHub {
		return fmt.Errorf("route %q: hubHost is required for mode %s", name, mode)
	}
	siteHost := strings.TrimSpace(rt.SiteHost)
	if siteHost == "" && mode == fleetv1alpha1.AppRouteModeSite {
		fallback := release
		if name != app.Name {
			fallback = name
		}
		siteHost = defaultRouteHostname(fallback, r.clusterEdgeDomain(ctx, app.Spec.ClusterName))
	}
	if siteHost == "" && mode == fleetv1alpha1.AppRouteModeSite {
		return fmt.Errorf("route %q: siteHost is required for mode %s", name, mode)
	}
	far := &fleetedge.FleetAppRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: routeNS,
			Labels:    map[string]string{"fleet.umkim.com/app": app.Name},
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, far, func() error {
		far.Labels = map[string]string{"fleet.umkim.com/app": app.Name}
		far.Spec = fleetedge.FleetAppRouteSpec{
			InstanceID:  name,
			Cluster:     app.Spec.ClusterName,
			Mode:        mode,
			HubHost:     hubHost,
			SiteHost:    siteHost,
			RequireAuth:       rt.RequireAuth,
			RequireAuthGroups: append([]string(nil), rt.RequireAuthGroups...),
			HealthPath:        rt.EffectiveHealthPath(),
			Service: fleetedge.ServiceRef{
				Namespace: rt.EffectiveServiceNamespace(app.Spec.EffectiveNamespace()),
				Name:      rt.EffectiveServiceName(release),
				Port:      rt.EffectiveServicePort(),
			},
		}
		return nil
	})
	return err
}

func (r *AppReconciler) deleteRoute(ctx context.Context, app *fleetv1alpha1.FleetApp) error {
	routeNS := r.Cfg.EdgeRoutesNamespace
	if routeNS == "" {
		routeNS = "fleet-system"
	}
	var existing fleetedge.FleetAppRouteList
	if err := r.List(ctx, &existing, client.InNamespace(routeNS), client.MatchingLabels{"fleet.umkim.com/app": app.Name}); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	for i := range existing.Items {
		if err := r.Delete(ctx, &existing.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	app.Status.RouteName = ""
	app.Status.RouteNames = nil
	return nil
}

func (r *AppReconciler) failApp(ctx context.Context, app *fleetv1alpha1.FleetApp, msg string) (ctrl.Result, error) {
	app.Status.Phase = fleetv1alpha1.AppPhaseFailed
	app.Status.Message = msg
	if err := r.Status().Update(ctx, app); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func appSpecHash(spec fleetv1alpha1.FleetAppSpec) string {
	b, _ := json.Marshal(spec)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:16])
}

func appJobName(prefix, name string) string {
	n := prefix + name
	if len(n) <= 63 {
		return n
	}
	sum := sha256.Sum256([]byte(name))
	return prefix + hex.EncodeToString(sum[:8])
}

func appValuesCMName(name string) string {
	n := "fa-vals-" + name
	if len(n) <= 63 {
		return n
	}
	sum := sha256.Sum256([]byte(name))
	return "fa-vals-" + hex.EncodeToString(sum[:8])
}

func appImageMapCMName(name string) string {
	n := "fa-imap-" + name
	if len(n) <= 63 {
		return n
	}
	sum := sha256.Sum256([]byte(name))
	return "fa-imap-" + hex.EncodeToString(sum[:8])
}

func (r *Reconciler) SetupAppWithManager(mgr ctrl.Manager) error {
	ar := &AppReconciler{Reconciler: r}
	return ctrl.NewControllerManagedBy(mgr).
		For(&fleetv1alpha1.FleetApp{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.ConfigMap{}).
		Complete(ar)
}

// appRouteHost returns the app's public host (site preferred, then hub).
func appRouteHost(app *fleetv1alpha1.FleetApp) string {
	for _, rt := range app.Spec.EffectiveRoutes() {
		if h := strings.TrimSpace(rt.SiteHost); h != "" {
			return h
		}
		if h := strings.TrimSpace(rt.HubHost); h != "" {
			return h
		}
	}
	return ""
}

// appOIDCClient builds the Authelia OIDC client an app declares, deriving
// redirect URIs from the app's route host.
func appOIDCClient(app *fleetv1alpha1.FleetApp) (fleetv1alpha1.IngressAuthOIDCClient, bool) {
	oc := app.Spec.OIDCClient
	if oc == nil {
		return fleetv1alpha1.IngressAuthOIDCClient{}, false
	}
	clientID := strings.TrimSpace(oc.ClientID)
	if clientID == "" {
		clientID = app.Name
	}
	name := strings.TrimSpace(oc.ClientName)
	if name == "" {
		name = clientID
	}
	host := appRouteHost(app)
	uris := []string{}
	for _, p := range oc.RedirectPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		if host != "" {
			uris = append(uris, "https://"+host+p)
		}
	}
	for _, u := range oc.ExtraRedirectURIs {
		if u = strings.TrimSpace(u); u != "" {
			uris = append(uris, u)
		}
	}
	return fleetv1alpha1.IngressAuthOIDCClient{
		ClientID:     clientID,
		ClientName:   name,
		Scopes:       oc.Scopes,
		RequirePKCE:  oc.RequirePKCE,
		RedirectURIs: uris,
		Owner:        app.Name,
	}, true
}

// ensureAppOIDCClient registers or refreshes the app's Authelia OIDC client on
// its cluster (owned by the app), leaving manual and other apps' clients alone.
func (r *AppReconciler) ensureAppOIDCClient(ctx context.Context, app *fleetv1alpha1.FleetApp) error {
	desired, ok := appOIDCClient(app)
	if !ok {
		return r.removeAppOIDCClient(ctx, app)
	}
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: app.Spec.ClusterName}, &fcj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	base := fcj.DeepCopy()
	clients := make([]fleetv1alpha1.IngressAuthOIDCClient, 0, len(fcj.Spec.IngressAuth.OIDCClients)+1)
	for _, c := range fcj.Spec.IngressAuth.OIDCClients {
		// Replace this app's own entry and any manual entry with the same
		// client id (the app takes ownership of that id).
		if c.Owner == app.Name || c.ClientID == desired.ClientID {
			continue
		}
		clients = append(clients, c)
	}
	clients = append(clients, desired)
	fcj.Spec.IngressAuth.OIDCClients = clients
	if reflect.DeepEqual(base.Spec.IngressAuth.OIDCClients, fcj.Spec.IngressAuth.OIDCClients) {
		return nil
	}
	if err := r.Patch(ctx, &fcj, client.MergeFrom(base)); err != nil {
		return err
	}
	return r.triggerIngressAuthReconcile(ctx)
}

// removeAppOIDCClient drops the app's owned OIDC clients from its cluster.
func (r *AppReconciler) removeAppOIDCClient(ctx context.Context, app *fleetv1alpha1.FleetApp) error {
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: app.Spec.ClusterName}, &fcj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	kept := make([]fleetv1alpha1.IngressAuthOIDCClient, 0, len(fcj.Spec.IngressAuth.OIDCClients))
	changed := false
	for _, c := range fcj.Spec.IngressAuth.OIDCClients {
		if c.Owner == app.Name {
			changed = true
			continue
		}
		kept = append(kept, c)
	}
	if !changed {
		return nil
	}
	base := fcj.DeepCopy()
	fcj.Spec.IngressAuth.OIDCClients = kept
	if err := r.Patch(ctx, &fcj, client.MergeFrom(base)); err != nil {
		return err
	}
	return r.triggerIngressAuthReconcile(ctx)
}

// triggerIngressAuthReconcile annotates FleetIngressAuth to run a reconcile.
func (r *AppReconciler) triggerIngressAuthReconcile(ctx context.Context) error {
	var fia fleetv1alpha1.FleetIngressAuth
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: fleetv1alpha1.FleetIngressAuthName}, &fia); err != nil {
		return client.IgnoreNotFound(err)
	}
	if fia.Annotations == nil {
		fia.Annotations = map[string]string{}
	}
	fia.Annotations[ingressauth.ReconcileAnnotation] = time.Now().UTC().Format(time.RFC3339)
	return r.Update(ctx, &fia)
}
