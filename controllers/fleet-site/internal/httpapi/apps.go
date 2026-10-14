package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/appchart"
	"github.com/rborso/umkim/controllers/fleet-site/internal/llm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type appPatchBody struct {
	Version    string                              `json:"version"`
	Values       json.RawMessage                     `json:"values"`
	ValuesYAML   string                              `json:"valuesYaml"`
	Namespace    string                              `json:"namespace"`
	Route        fleetv1alpha1.FleetAppRouteIntent   `json:"route"`
	Routes       []fleetv1alpha1.FleetAppRouteIntent `json:"routes,omitempty"`
	LlmBackend   *bool                               `json:"llmBackend,omitempty"`
	OIDCClientID string                              `json:"oidcClientId,omitempty"`
	OIDCClient   *fleetv1alpha1.FleetAppOIDCClient   `json:"oidcClient,omitempty"`
}

type appCreateBody struct {
	Name         string                              `json:"name"`
	ClusterName  string                              `json:"clusterName"`
	Chart        string                              `json:"chart"`
	Version      string                              `json:"version"`
	ReleaseName  string                              `json:"releaseName"`
	Namespace    string                              `json:"namespace"`
	Values       json.RawMessage                     `json:"values"`
	ValuesYAML   string                              `json:"valuesYaml"`
	Route        fleetv1alpha1.FleetAppRouteIntent   `json:"route"`
	Routes       []fleetv1alpha1.FleetAppRouteIntent `json:"routes,omitempty"`
	LlmBackend   *bool                               `json:"llmBackend,omitempty"`
	OIDCClientID string                              `json:"oidcClientId,omitempty"`
	OIDCClient   *fleetv1alpha1.FleetAppOIDCClient   `json:"oidcClient,omitempty"`
}

type appResponse struct {
	Name        string                              `json:"name"`
	ClusterName string                              `json:"clusterName"`
	Chart       string                              `json:"chart"`
	Version     string                              `json:"version,omitempty"`
	ReleaseName string                              `json:"releaseName"`
	Namespace   string                              `json:"namespace"`
	ValuesYAML  string                              `json:"valuesYaml,omitempty"`
	Route       fleetv1alpha1.FleetAppRouteIntent   `json:"route"`
	Routes      []fleetv1alpha1.FleetAppRouteIntent `json:"routes,omitempty"`
	Phase       string                              `json:"phase"`
	Message     string                              `json:"message,omitempty"`
	RouteName   string                              `json:"routeName,omitempty"`
	RouteNames  []string                            `json:"routeNames,omitempty"`
	LlmBackend  bool                                `json:"llmBackend,omitempty"`
}

type appsListResponse struct {
	Apps     []appResponse `json:"apps"`
	Defaults appsDefaults  `json:"defaults"`
}

type appsDefaults struct {
	ChartPrefix   string `json:"chartPrefix"`
	HubEdgeDomain string `json:"hubEdgeDomain,omitempty"`
}

func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listApps(w, r)
	case http.MethodPost:
		s.createApp(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAppByName(w http.ResponseWriter, r *http.Request, name string) {
	switch r.Method {
	case http.MethodGet:
		s.getApp(w, r, name)
	case http.MethodDelete:
		s.deleteApp(w, r, name)
	case http.MethodPatch:
		s.patchApp(w, r, name)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listApps(w http.ResponseWriter, r *http.Request) {
	var list fleetv1alpha1.FleetAppList
	if err := s.Client.List(r.Context(), &list, client.InNamespace(s.Cfg.Namespace)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sortByName(list.Items, func(a fleetv1alpha1.FleetApp) string { return a.Name })
	out := make([]appResponse, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, appToResponse(&list.Items[i]))
	}
	writeJSON(w, http.StatusOK, appsListResponse{Apps: out, Defaults: s.appsDefaults()})
}

func (s *Server) getApp(w http.ResponseWriter, r *http.Request, name string) {
	var app fleetv1alpha1.FleetApp
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &app); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, appToResponse(&app))
}

func (s *Server) createApp(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	var body appCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	cluster := strings.TrimSpace(body.ClusterName)
	if cluster == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "clusterName is required; choose the Ready cluster to install on"})
		return
	}
	if !appchart.ValidDNS1123(cluster) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "clusterName must be a DNS-1123 label"})
		return
	}
	if s.Cfg.ManagementClusterName != "" && cluster == s.Cfg.ManagementClusterName {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the hub cluster is not a valid FleetApp target"})
		return
	}
	fcj, err := s.findFleetClusterJoin(r.Context(), cluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cluster " + cluster + " not found"})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "cluster " + cluster + " is not Ready (phase " + fcj.Status.Phase + ")"})
		return
	}
	chart := appchart.ExpandChart(body.Chart, s.Cfg.DefaultChartPrefix())
	if err := appchart.ChartAllowed(chart, s.Cfg.ChartOCIPrefixes()); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	release := strings.TrimSpace(body.ReleaseName)
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = release
	}
	if name == "" {
		name = cluster + "-app"
		if i := strings.LastIndex(chart, "/"); i >= 0 && i+1 < len(chart) {
			name = chart[i+1:]
		}
	}
	if !appchart.ValidDNS1123(name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name must be a DNS-1123 label (lowercase)"})
		return
	}
	if release == "" {
		release = name
	}
	if !appchart.ValidDNS1123(release) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "releaseName must be a DNS-1123 label"})
		return
	}
	ns := strings.TrimSpace(body.Namespace)
	if ns == "" {
		ns = "default"
	}
	if !appchart.ValidDNS1123(ns) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "namespace must be a DNS-1123 label"})
		return
	}
	if !appchart.ValidVersion(body.Version) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid chart version"})
		return
	}
	mode := body.Route.Mode
	if !appchart.ValidRouteMode(mode) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "route.mode must be none, hub, or site"})
		return
	}
	if msg := validateRouteProtocols(body.Route, body.Routes); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	for _, rt := range body.Routes {
		if !appchart.ValidRouteMode(rt.Mode) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "routes[].mode must be none, hub, or site"})
			return
		}
	}
	valuesYAML := strings.TrimSpace(body.ValuesYAML)
	if valuesYAML == "" && len(body.Values) > 0 && string(body.Values) != "null" {
		y, err := jsonValuesToYAML(body.Values)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "values: " + err.Error()})
			return
		}
		valuesYAML = y
	}
	app := &fleetv1alpha1.FleetApp{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.Cfg.Namespace},
		Spec: fleetv1alpha1.FleetAppSpec{
			ClusterName: cluster,
			Chart:       chart,
			Version:     strings.TrimSpace(body.Version),
			ReleaseName: release,
			Namespace:   ns,
			ValuesYAML:   valuesYAML,
			Route:        body.Route,
			Routes:       body.Routes,
			OIDCClientID: strings.TrimSpace(body.OIDCClientID),
			OIDCClient:   body.OIDCClient,
		},
	}
	if len(app.Spec.Routes) == 0 && app.Spec.Route.Mode == "" {
		app.Spec.Route.Mode = fleetv1alpha1.AppRouteModeNone
	}
	if msg := s.validateAppRouteHosts(r.Context(), cluster, append([]fleetv1alpha1.FleetAppRouteIntent{body.Route}, body.Routes...)); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	if msg := s.findRouteHostConflict(r.Context(), routeHosts(body.Route, body.Routes), ""); msg != "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": msg})
		return
	}
	if body.LlmBackend != nil && llm.IsLlamaChart(chart) {
		app.SetLLMBackendAnnotation(*body.LlmBackend)
	}
	if err := s.Client.Create(r.Context(), app); err != nil {
		if apierrors.IsAlreadyExists(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "app " + name + " already exists"})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, appToResponse(app))
}

func (s *Server) patchApp(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	var body appPatchBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	var app fleetv1alpha1.FleetApp
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &app); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch app.Status.Phase {
	case fleetv1alpha1.AppPhaseInstalling, fleetv1alpha1.AppPhaseDeleting:
		writeJSON(w, http.StatusConflict, map[string]string{"error": "app is " + app.Status.Phase + "; wait for a terminal phase before patching"})
		return
	}
	changed := false
	if v := strings.TrimSpace(body.Version); v != "" {
		if !appchart.ValidVersion(v) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid chart version"})
			return
		}
		if app.Spec.Version != v {
			app.Spec.Version = v
			changed = true
		}
	}
	if ns := strings.TrimSpace(body.Namespace); ns != "" {
		if !appchart.ValidDNS1123(ns) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "namespace must be a DNS-1123 label"})
			return
		}
		if app.Spec.Namespace != ns {
			app.Spec.Namespace = ns
			changed = true
		}
	}
	valuesYAML := strings.TrimSpace(body.ValuesYAML)
	if valuesYAML == "" && len(body.Values) > 0 && string(body.Values) != "null" {
		y, err := jsonValuesToYAML(body.Values)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "values: " + err.Error()})
			return
		}
		valuesYAML = y
	}
	if valuesYAML != "" && app.Spec.ValuesYAML != valuesYAML {
		app.Spec.ValuesYAML = valuesYAML
		changed = true
	}
	if len(body.Routes) > 0 {
		for _, rt := range body.Routes {
			if !appchart.ValidRouteMode(rt.Mode) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "routes[].mode must be none, hub, or site"})
				return
			}
		}
		if msg := validateRouteProtocols(fleetv1alpha1.FleetAppRouteIntent{}, body.Routes); msg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
			return
		}
		app.Spec.Routes = body.Routes
		app.Spec.Route = fleetv1alpha1.FleetAppRouteIntent{}
		changed = true
	} else if body.Route.Mode != "" {
		if !appchart.ValidRouteMode(body.Route.Mode) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "route.mode must be none, hub, or site"})
			return
		}
		app.Spec.Route = body.Route
		changed = true
	} else if body.Route.HubHost != "" || body.Route.SiteHost != "" {
		if body.Route.HubHost != "" {
			app.Spec.Route.HubHost = body.Route.HubHost
		}
		if body.Route.SiteHost != "" {
			app.Spec.Route.SiteHost = body.Route.SiteHost
		}
		changed = true
	}
	if body.Route.ServiceName != "" {
		app.Spec.Route.ServiceName = body.Route.ServiceName
		changed = true
	}
	if body.Route.ServicePort > 0 {
		app.Spec.Route.ServicePort = body.Route.ServicePort
		changed = true
	}
	if body.Route.ServiceNamespace != "" {
		app.Spec.Route.ServiceNamespace = body.Route.ServiceNamespace
		changed = true
	}
	if body.OIDCClient != nil || strings.TrimSpace(body.OIDCClientID) != "" {
		app.Spec.OIDCClient = body.OIDCClient
		app.Spec.OIDCClientID = strings.TrimSpace(body.OIDCClientID)
		changed = true
	}
	if body.LlmBackend != nil && llm.IsLlamaChart(app.Spec.Chart) {
		app.SetLLMBackendAnnotation(*body.LlmBackend)
		changed = true
	}
	if !changed {
		writeJSON(w, http.StatusOK, appToResponse(&app))
		return
	}
	if msg := s.validateAppRouteHosts(r.Context(), app.Spec.ClusterName, append([]fleetv1alpha1.FleetAppRouteIntent{app.Spec.Route}, app.Spec.Routes...)); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	if msg := s.findRouteHostConflict(r.Context(), routeHosts(app.Spec.Route, app.Spec.Routes), app.Name); msg != "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": msg})
		return
	}
	if err := s.Client.Update(r.Context(), &app); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, appToResponse(&app))
}

func (s *Server) deleteApp(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	var app fleetv1alpha1.FleetApp
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &app); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.Client.Delete(r.Context(), &app); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"name": name, "phase": fleetv1alpha1.AppPhaseDeleting})
}

func (s *Server) appsDefaults() appsDefaults {
	return appsDefaults{
		ChartPrefix:   s.Cfg.DefaultChartPrefix(),
		HubEdgeDomain: s.Cfg.HubEdgeDomain,
	}
}

func appToResponse(app *fleetv1alpha1.FleetApp) appResponse {
	return appResponse{
		Name:        app.Name,
		ClusterName: app.Spec.ClusterName,
		Chart:       app.Spec.Chart,
		Version:     app.Spec.Version,
		ReleaseName: app.Spec.EffectiveReleaseName(app.Name),
		Namespace:   app.Spec.EffectiveNamespace(),
		ValuesYAML:  app.Spec.ValuesYAML,
		Route:       app.Spec.Route,
		Routes:      app.Spec.Routes,
		Phase:       app.Status.Phase,
		Message:     app.Status.Message,
		RouteName:   app.Status.RouteName,
		RouteNames:  app.Status.RouteNames,
		LlmBackend:  app.RegistersLLMBackend(),
	}
}

func jsonValuesToYAML(raw json.RawMessage) (string, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		return strings.TrimSpace(s), nil
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	b, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// routeHosts returns the hub/site hostnames an app's route(s) publish. When
// routes[] is set it takes precedence over the single route.
func routeHosts(route fleetv1alpha1.FleetAppRouteIntent, routes []fleetv1alpha1.FleetAppRouteIntent) []string {
	var hosts []string
	add := func(rt fleetv1alpha1.FleetAppRouteIntent) {
		if h := strings.TrimSpace(rt.HubHost); h != "" {
			hosts = append(hosts, h)
		}
		if h := strings.TrimSpace(rt.SiteHost); h != "" {
			hosts = append(hosts, h)
		}
	}
	if len(routes) > 0 {
		for _, rt := range routes {
			add(rt)
		}
		return hosts
	}
	add(route)
	return hosts
}

// findRouteHostConflict returns a clear message when another FleetApp already
// publishes one of hosts. excludeName skips the app being updated.
func (s *Server) findRouteHostConflict(ctx context.Context, hosts []string, excludeName string) string {
	if len(hosts) == 0 {
		return ""
	}
	want := map[string]string{}
	for _, h := range hosts {
		want[strings.ToLower(strings.TrimSpace(h))] = h
	}
	var list fleetv1alpha1.FleetAppList
	if err := s.Client.List(ctx, &list, client.InNamespace(s.Cfg.Namespace)); err != nil {
		return ""
	}
	for i := range list.Items {
		other := &list.Items[i]
		if other.Name == excludeName {
			continue
		}
		for _, h := range routeHosts(other.Spec.Route, other.Spec.Routes) {
			if orig, ok := want[strings.ToLower(h)]; ok {
				return fmt.Sprintf("host %q is already used by app %q", orig, other.Name)
			}
		}
	}
	return ""
}

// validateRouteProtocols checks route protocols and the tcp forwardPort
// requirement, returning an operator-facing message ("" when valid).
func validateRouteProtocols(route fleetv1alpha1.FleetAppRouteIntent, routes []fleetv1alpha1.FleetAppRouteIntent) string {
	all := append([]fleetv1alpha1.FleetAppRouteIntent{route}, routes...)
	for _, rt := range all {
		if !appchart.ValidRouteProtocol(rt.Protocol) {
			return "route protocol must be http or tcp"
		}
		if rt.EffectiveProtocol() == fleetv1alpha1.AppRouteProtocolTCP && rt.ForwardPort <= 0 {
			return "route forwardPort is required for protocol tcp"
		}
	}
	return ""
}
