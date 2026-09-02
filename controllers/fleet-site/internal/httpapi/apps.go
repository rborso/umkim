package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/appchart"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type appCreateBody struct {
	Name        string                            `json:"name"`
	ClusterName string                            `json:"clusterName"`
	Chart       string                            `json:"chart"`
	Version     string                            `json:"version"`
	ReleaseName string                            `json:"releaseName"`
	Namespace   string                            `json:"namespace"`
	Values      json.RawMessage                   `json:"values"`
	ValuesYAML  string                            `json:"valuesYaml"`
	Route       fleetv1alpha1.FleetAppRouteIntent `json:"route"`
}

type appResponse struct {
	Name        string                            `json:"name"`
	ClusterName string                            `json:"clusterName"`
	Chart       string                            `json:"chart"`
	Version     string                            `json:"version,omitempty"`
	ReleaseName string                            `json:"releaseName"`
	Namespace   string                            `json:"namespace"`
	ValuesYAML  string                            `json:"valuesYaml,omitempty"`
	Route       fleetv1alpha1.FleetAppRouteIntent `json:"route"`
	Phase       string                            `json:"phase"`
	Message     string                            `json:"message,omitempty"`
	RouteName   string                            `json:"routeName,omitempty"`
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
			ValuesYAML:  valuesYAML,
			Route:       body.Route,
		},
	}
	if app.Spec.Route.Mode == "" {
		app.Spec.Route.Mode = fleetv1alpha1.AppRouteModeNone
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
		Phase:       app.Status.Phase,
		Message:     app.Status.Message,
		RouteName:   app.Status.RouteName,
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
