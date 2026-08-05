package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	fleetedge "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type instanceHubRouteResponse struct {
	InstanceID          string `json:"instance_id"`
	EffectiveMode       string `json:"effective_mode,omitempty"`
	HubRouteApplicable  bool   `json:"hub_route_applicable"`
	HubHost             string `json:"hub_host,omitempty"`
	ActiveCluster       string `json:"active_cluster,omitempty"`
	StandbyCluster      string `json:"standby_cluster,omitempty"`
	HubRouteHealthy     *bool  `json:"hub_route_healthy,omitempty"`
	FailoverPhase       string `json:"failover_phase,omitempty"`
	FailoverMode        string `json:"failover_mode,omitempty"`
	LastFailoverAt      string `json:"last_failover_at,omitempty"`
	ConsecutiveFailures int    `json:"consecutive_failures,omitempty"`
}

func routeEffectiveMode(route *fleetedge.FleetAppRoute) string {
	mode := route.Spec.EffectiveMode
	if mode == "" {
		mode = route.Spec.Mode
	}
	return mode
}

func hubRouteApplicable(mode string) bool {
	return mode == fleetedge.RouteModeHub || mode == fleetedge.RouteModeDual
}

func (s *Server) handleInstanceHubRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	instanceID := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/instance-routes/")
	instanceID = strings.Trim(instanceID, "/")
	if instanceID == "" {
		http.Error(w, "instance id required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var list fleetedge.FleetAppRouteList
	if err := s.Client.List(ctx, &list, client.InNamespace(s.Cfg.EdgeRoutesNamespace)); err != nil {
		if meta.IsNoMatchError(err) {
			http.Error(w, "FleetAppRoute CRD not installed", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var found *fleetedge.FleetAppRoute
	for i := range list.Items {
		route := &list.Items[i]
		if route.Spec.InstanceID == instanceID ||
			route.Labels["edge.umkim.com/instance-id"] == instanceID {
			found = route
			break
		}
	}
	if found == nil {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}

	lastFailover := ""
	if found.Annotations != nil {
		lastFailover = found.Annotations["fleet.umkim.com/last-failover-at"]
	}
	effectiveMode := routeEffectiveMode(found)
	resp := instanceHubRouteResponse{
		InstanceID:          instanceID,
		EffectiveMode:       effectiveMode,
		HubRouteApplicable:  hubRouteApplicable(effectiveMode),
		HubHost:             found.Spec.HubHost,
		ActiveCluster:       found.ActiveClusterName(),
		StandbyCluster:      found.Spec.StandbyCluster,
		FailoverPhase:       found.Status.FailoverPhase,
		FailoverMode:        found.Spec.FailoverMode,
		LastFailoverAt:      lastFailover,
		ConsecutiveFailures: found.Status.ConsecutiveFailures,
	}
	if resp.HubRouteApplicable {
		healthy := found.Status.HubPathHealthy
		resp.HubRouteHealthy = &healthy
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
