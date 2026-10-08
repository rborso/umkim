package v1alpha1

import "testing"

func TestEffectiveRoutesLegacySingle(t *testing.T) {
	s := FleetAppSpec{Route: FleetAppRouteIntent{Mode: "site", SiteHost: "a.example.com"}}
	rs := s.EffectiveRoutes()
	if len(rs) != 1 || rs[0].SiteHost != "a.example.com" {
		t.Fatalf("legacy route = %#v", rs)
	}
}

func TestEffectiveRoutesMultipleWins(t *testing.T) {
	s := FleetAppSpec{
		Route: FleetAppRouteIntent{Mode: "site", SiteHost: "ignored.example.com"},
		Routes: []FleetAppRouteIntent{
			{Key: "sonarr", Mode: "site", SiteHost: "sonarr.example.com", ServicePort: 8989, RequireAuth: true},
			{Key: "radarr", Mode: "site", SiteHost: "radarr.example.com"},
		},
	}
	rs := s.EffectiveRoutes()
	if len(rs) != 2 {
		t.Fatalf("routes = %#v", rs)
	}
	if rs[0].RouteKey(rs[0].ServiceName) != "sonarr" || !rs[0].RequireAuth {
		t.Fatalf("route[0] = %#v", rs[0])
	}
	if rs[0].EffectiveServicePort() != 8989 {
		t.Fatalf("route[0] port = %d", rs[0].EffectiveServicePort())
	}
	if rs[1].EffectiveServicePort() != 80 || rs[1].EffectiveHealthPath() != "/" {
		t.Fatalf("route[1] defaults = %#v", rs[1])
	}
	if rs[1].EffectiveServiceName("fallback") != "fallback" {
		t.Fatalf("route[1] service fallback = %q", rs[1].EffectiveServiceName("fallback"))
	}
}

func TestEffectiveRoutesEmpty(t *testing.T) {
	if got := (FleetAppSpec{}).EffectiveRoutes(); got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}

func TestEffectiveModeDefault(t *testing.T) {
	if got := (FleetAppRouteIntent{}).EffectiveMode(); got != AppRouteModeNone {
		t.Fatalf("default mode = %q", got)
	}
}
