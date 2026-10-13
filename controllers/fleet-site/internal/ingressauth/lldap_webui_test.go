package ingressauth

import (
	"context"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHubLLDAPWebUIRoutesYAMLGatedByAuthelia(t *testing.T) {
	y := hubLLDAPWebUIRoutesYAML("lldap.demo.example", "fleet-console")
	for _, want := range []string{
		"Host(`lldap.demo.example`)",
		"entryPoints:\n        - websecure",
		"- fleet-edge-authelia",
		"url: http://lldap-webui.fleet-console.svc.cluster.local:17170",
		"certResolver: letsencrypt",
	} {
		if !strings.Contains(y, want) {
			t.Fatalf("rendered route missing %q:\n%s", want, y)
		}
	}
}

func TestEnsureHubLLDAPWebUIRoutePublishesAndWithdraws(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-traefik-dynamic", Namespace: "fleet-ingress"},
		Data:       map[string]string{"routes.yml": hubDynamicCMPlaceholder},
	}
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: "fleet-ingress"},
		Spec:       appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm, dep).Build()
	cfg := config.Config{HubGatewayNamespace: "fleet-ingress", HubGatewayDeployment: "gateway", HubDynamicCMName: "fleet-traefik-dynamic", ConsoleNamespace: "fleet-console"}

	if err := ensureHubLLDAPWebUIRoute(context.Background(), c, cfg, "lldap.demo.example", true); err != nil {
		t.Fatal(err)
	}
	var got corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "fleet-ingress", Name: "fleet-traefik-dynamic"}, &got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Data["routes.yml"], "fleet-lldap-webui-tls") {
		t.Fatalf("route not published:\n%s", got.Data["routes.yml"])
	}
	// The gateway must be asked to reload (file watch is disabled).
	var gotDep appsv1.Deployment
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "fleet-ingress", Name: "gateway"}, &gotDep); err != nil {
		t.Fatal(err)
	}
	if gotDep.Spec.Template.Annotations[hubGatewayReloadAnnot] == "" {
		t.Fatal("gateway reload annotation not set")
	}

	if err := ensureHubLLDAPWebUIRoute(context.Background(), c, cfg, "lldap.demo.example", false); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "fleet-ingress", Name: "fleet-traefik-dynamic"}, &got); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Data["routes.yml"], "fleet-lldap-webui") {
		t.Fatalf("route not withdrawn:\n%s", got.Data["routes.yml"])
	}
}

func TestLLDAPWebUIHostForFIA(t *testing.T) {
	fia := &fleetv1alpha1.FleetIngressAuth{Spec: fleetv1alpha1.FleetIngressAuthSpec{CookieDomain: "demo.example"}}
	if got := lldapWebUIHostForFIA(fia); got != "lldap.demo.example" {
		t.Fatalf("cookie domain host = %q", got)
	}
	fia2 := &fleetv1alpha1.FleetIngressAuth{Spec: fleetv1alpha1.FleetIngressAuthSpec{AccessDomains: []string{"*.demo.example", "demo.example"}}}
	if got := lldapWebUIHostForFIA(fia2); got != "lldap.demo.example" {
		t.Fatalf("access domain host = %q", got)
	}
}
