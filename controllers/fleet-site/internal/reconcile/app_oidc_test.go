package reconcile

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAppOIDCClient(t *testing.T) {
	f := false
	paperless := &fleetv1alpha1.FleetApp{
		ObjectMeta: metav1.ObjectMeta{Name: "paperless"},
		Spec: fleetv1alpha1.FleetAppSpec{
			ClusterName: "c1",
			Routes:      []fleetv1alpha1.FleetAppRouteIntent{{Mode: "site", SiteHost: "paperless.example.com"}},
			OIDCClient: &fleetv1alpha1.FleetAppOIDCClient{
				RedirectPaths: []string{"/accounts/oidc/authelia/login/callback/"},
				RequirePKCE:   &f,
			},
		},
	}
	c, ok := appOIDCClient(paperless)
	if !ok {
		t.Fatal("expected a client")
	}
	if c.ClientID != "paperless" || c.Owner != "paperless" {
		t.Fatalf("id/owner = %q/%q", c.ClientID, c.Owner)
	}
	if len(c.RedirectURIs) != 1 || c.RedirectURIs[0] != "https://paperless.example.com/accounts/oidc/authelia/login/callback/" {
		t.Fatalf("redirect uris = %v", c.RedirectURIs)
	}
	if c.RequirePKCE == nil || *c.RequirePKCE {
		t.Fatalf("requirePkce should be false")
	}

	immich := &fleetv1alpha1.FleetApp{
		ObjectMeta: metav1.ObjectMeta{Name: "immich"},
		Spec: fleetv1alpha1.FleetAppSpec{
			ClusterName: "c1",
			Routes:      []fleetv1alpha1.FleetAppRouteIntent{{Mode: "site", SiteHost: "immich.example.com"}},
			OIDCClient: &fleetv1alpha1.FleetAppOIDCClient{
				ClientID:          "immich",
				RedirectPaths:     []string{"/auth/login", "/user-settings"},
				ExtraRedirectURIs: []string{"app.immich:///oauth-callback"},
			},
		},
	}
	c2, _ := appOIDCClient(immich)
	if len(c2.RedirectURIs) != 3 {
		t.Fatalf("immich redirect uris = %v", c2.RedirectURIs)
	}
	if c2.RedirectURIs[0] != "https://immich.example.com/auth/login" {
		t.Fatalf("immich first uri = %q", c2.RedirectURIs[0])
	}
	if c2.RequirePKCE != nil {
		t.Fatalf("immich requirePkce should be unset (defaults true)")
	}

	noClient := &fleetv1alpha1.FleetApp{ObjectMeta: metav1.ObjectMeta{Name: "x"}}
	if _, ok := appOIDCClient(noClient); ok {
		t.Fatal("expected no client when OIDCClient is nil")
	}
}
