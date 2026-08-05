package routesync

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
)

func TestParseRequireAuth(t *testing.T) {
	tests := []struct {
		name string
		uv   map[string]string
		mode string
		want bool
	}{
		{"hub absent", map[string]string{}, fleetv1alpha1.RouteModeHub, false},
		{"hub true", map[string]string{"routing.require_auth": "true"}, fleetv1alpha1.RouteModeHub, true},
		{"hub false", map[string]string{"routing.require_auth": "false"}, fleetv1alpha1.RouteModeHub, false},
		{"site true", map[string]string{"routing.require_auth": "1"}, fleetv1alpha1.RouteModeSite, true},
		{"dual forced off", map[string]string{"routing.require_auth": "true"}, fleetv1alpha1.RouteModeDual, false},
		{"none forced off", map[string]string{"routing.require_auth": "true"}, fleetv1alpha1.RouteModeNone, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseRequireAuth(tc.uv, tc.mode); got != tc.want {
				t.Fatalf("parseRequireAuth() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUserValueAsString(t *testing.T) {
	if got := userValueAsString(userValue{Path: "routing.require_auth", Value: true}); got != "true" {
		t.Fatalf("bool true: %q", got)
	}
	if got := userValueAsString(userValue{Path: "routing.require_auth", Value: false}); got != "false" {
		t.Fatalf("bool false: %q", got)
	}
	if got := userValueAsString(userValue{Path: "routing.mode", Value: "hub"}); got != "hub" {
		t.Fatalf("string: %q", got)
	}
}
