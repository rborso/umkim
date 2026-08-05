package httpapi

import (
	"testing"

	fleetedge "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
)

func TestHubRouteApplicable(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{fleetedge.RouteModeHub, true},
		{fleetedge.RouteModeDual, true},
		{fleetedge.RouteModeSite, false},
		{fleetedge.RouteModeNone, false},
		{"", false},
	}
	for _, tc := range tests {
		if got := hubRouteApplicable(tc.mode); got != tc.want {
			t.Fatalf("hubRouteApplicable(%q) = %v, want %v", tc.mode, got, tc.want)
		}
	}
}

func TestRouteEffectiveMode(t *testing.T) {
	route := &fleetedge.FleetAppRoute{}
	route.Spec.Mode = fleetedge.RouteModeSite
	if got := routeEffectiveMode(route); got != fleetedge.RouteModeSite {
		t.Fatalf("expected mode from spec.mode, got %q", got)
	}
	route.Spec.EffectiveMode = fleetedge.RouteModeHub
	if got := routeEffectiveMode(route); got != fleetedge.RouteModeHub {
		t.Fatalf("expected effectiveMode to win, got %q", got)
	}
}
