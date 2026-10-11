package reconcile

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingressauth"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestAppOIDCClientSecretReady(t *testing.T) {
	ns := "fleet-console"
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	app := &fleetv1alpha1.FleetApp{
		ObjectMeta: metav1.ObjectMeta{Name: "immich", Namespace: ns},
		Spec: fleetv1alpha1.FleetAppSpec{
			ClusterName: "lab-ha-0",
			Routes:      []fleetv1alpha1.FleetAppRouteIntent{{Mode: "site", SiteHost: "immich.example.com"}},
			OIDCClient:  &fleetv1alpha1.FleetAppOIDCClient{ClientID: "immich", RedirectPaths: []string{"/auth/login"}},
		},
	}
	secretName := ingressauth.ClusterSecretName("lab-ha-0")
	reconciler := func(objs ...client.Object) *AppReconciler {
		return &AppReconciler{Reconciler: &Reconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
			Scheme: scheme,
			Cfg:    config.Config{Namespace: ns},
		}}
	}
	ctx := context.Background()

	// No cluster ingress-auth secret yet: not ready.
	if ready, err := reconciler().appOIDCClientSecretReady(ctx, app); err != nil || ready {
		t.Fatalf("no secret: ready=%v err=%v", ready, err)
	}

	// Secret exists but has no generated entry for this client: not ready.
	other := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
		Data:       map[string][]byte{"oidcClientSecrets": []byte(`{"paperless":"x"}`)},
	}
	if ready, err := reconciler(other).appOIDCClientSecretReady(ctx, app); err != nil || ready {
		t.Fatalf("missing client entry: ready=%v err=%v", ready, err)
	}

	// Generated secret present: ready.
	generated := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
		Data:       map[string][]byte{"oidcClientSecrets": []byte(`{"immich":"s3cr3t"}`)},
	}
	if ready, err := reconciler(generated).appOIDCClientSecretReady(ctx, app); err != nil || !ready {
		t.Fatalf("generated secret: ready=%v err=%v", ready, err)
	}

	// App without an OIDC client is always ready.
	plain := &fleetv1alpha1.FleetApp{ObjectMeta: metav1.ObjectMeta{Name: "whoami", Namespace: ns}}
	if ready, err := reconciler().appOIDCClientSecretReady(ctx, plain); err != nil || !ready {
		t.Fatalf("no oidc client: ready=%v err=%v", ready, err)
	}
}
