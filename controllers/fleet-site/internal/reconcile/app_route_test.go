package reconcile

import "testing"

func TestDefaultRouteHostname(t *testing.T) {
	if got := defaultRouteHostname("whoami", "homelab.example.com"); got != "whoami.homelab.example.com" {
		t.Fatalf("got %q", got)
	}
	if defaultRouteHostname("", "x.test") != "" {
		t.Fatal("expected empty without release")
	}
	if defaultRouteHostname("app", "") != "" {
		t.Fatal("expected empty without domain")
	}
}
