package reconcile

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestRouteObjectName(t *testing.T) {
	if got := routeObjectName("servarr", 0, fleetv1alpha1.FleetAppRouteIntent{Key: "sonarr"}); got != "servarr" {
		t.Fatalf("first route name = %q, want servarr", got)
	}
	if got := routeObjectName("servarr", 1, fleetv1alpha1.FleetAppRouteIntent{Key: "Sonarr 4K"}); got != "servarr-sonarr-4k" {
		t.Fatalf("second route name = %q", got)
	}
	if got := routeObjectName("servarr", 2, fleetv1alpha1.FleetAppRouteIntent{ServiceName: "qbittorrent-web"}); got != "servarr-qbittorrent-web" {
		t.Fatalf("third route name = %q", got)
	}
	if got := routeObjectName("servarr", 3, fleetv1alpha1.FleetAppRouteIntent{}); got != "servarr-r3" {
		t.Fatalf("unnamed route name = %q", got)
	}
}

func TestSanitizeRouteKey(t *testing.T) {
	cases := map[string]string{
		"Sonarr 4K":    "sonarr-4k",
		"  qbit-web  ": "qbit-web",
		"a/b/c":        "a-b-c",
		"---x---":      "x",
		"":             "",
	}
	for in, want := range cases {
		if got := sanitizeRouteKey(in); got != want {
			t.Fatalf("sanitizeRouteKey(%q) = %q, want %q", in, got, want)
		}
	}
}
