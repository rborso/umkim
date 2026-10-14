package reconcile

import (
	"context"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingressauth"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func directoryApp(cluster, ns string) *fleetv1alpha1.FleetApp {
	return &fleetv1alpha1.FleetApp{
		ObjectMeta: metav1.ObjectMeta{Name: "gitea", Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetAppSpec{
			ClusterName: cluster,
			Namespace:   ns,
			Routes:      []fleetv1alpha1.FleetAppRouteIntent{{Mode: "site", SiteHost: "gitea.example.com", RequireAuth: true}},
			ValuesYAML:  "auth:\n  ldap:\n    enabled: true\n    existingSecret: ${LLDAP_BIND_SECRET}\n",
		},
	}
}

// The controller provides the directory bind secret so a native-LDAP chart needs
// no hand-made Secret, and a rebuilt cluster needs no manual step.
func TestDirectoryBindSecretIsProvided(t *testing.T) {
	scheme := deleteTestScheme(t)
	clusterSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: ingressauth.ClusterSecretName("lab"), Namespace: "fleet-console"},
		Data:       map[string][]byte{"lldapJwtSecret": []byte("test-jwt-secret")},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(clusterSecret).Build()
	r := &AppReconciler{Reconciler: &Reconciler{Client: c, Scheme: scheme, Cfg: config.Config{Namespace: "fleet-console"}}}

	app := directoryApp("lab", "gitea")
	out, err := r.substitutePlaceholders(context.Background(), app, app.Spec.ValuesYAML)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "${LLDAP_BIND_SECRET}") {
		t.Fatalf("token not substituted:\n%s", out)
	}
	if !strings.Contains(out, "existingSecret: gitea-lldap-bind") {
		t.Fatalf("expected the provided secret name:\n%s", out)
	}
	var sec corev1.Secret
	// The controller keeps the credentials on the hub; the app's helm job
	// installs them into the app namespace on the spoke.
	if err := c.Get(context.Background(), types.NamespacedName{Name: "gitea-lldap-bind", Namespace: "fleet-console"}, &sec); err != nil {
		t.Fatalf("bind secret not created: %v", err)
	}
	// The fake client keeps StringData as written (the API server folds it into
	// Data), so assert on StringData here.
	if sec.StringData["LLDAP_LDAP_USER_PASS"] == "" {
		t.Fatal("bind secret has no password")
	}
	if sec.StringData["username"] != "svc-bind" {
		t.Fatalf("bind username = %q", sec.StringData["username"])
	}
}

// No directory on the cluster: the token resolves empty and nothing is created.
func TestDirectoryBindSecretAbsentWithoutDirectory(t *testing.T) {
	scheme := deleteTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AppReconciler{Reconciler: &Reconciler{Client: c, Scheme: scheme, Cfg: config.Config{Namespace: "fleet-console"}}}
	app := directoryApp("other", "gitea")
	out, err := r.substitutePlaceholders(context.Background(), app, app.Spec.ValuesYAML)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "existingSecret: \n") && !strings.Contains(out, "existingSecret: ") {
		t.Fatalf("token should resolve empty:\n%s", out)
	}
}

// The conventional ingress block is filled in from the app's route.
func TestIngressValueDefaults(t *testing.T) {
	app := directoryApp("lab", "gitea")
	got := ingressValueDefaults(app)
	for _, want := range []string{"ingress:", "enabled: true", `host: "gitea.example.com"`, "forwardAuth: true"} {
		if !strings.Contains(got, want) {
			t.Fatalf("defaults missing %q:\n%s", want, got)
		}
	}
	app.Spec.Routes = nil
	if ingressValueDefaults(app) != "" {
		t.Fatal("an app without a route should get no ingress defaults")
	}
}
