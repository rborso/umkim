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

func TestEffectiveHubLldapWebUIExposure(t *testing.T) {
	cases := []struct {
		spec fleetv1alpha1.FleetIngressAuthSpec
		want string
	}{
		{fleetv1alpha1.FleetIngressAuthSpec{}, "disabled"},
		{fleetv1alpha1.FleetIngressAuthSpec{HubExposeLldapWebUI: true}, "authelia"},
		{fleetv1alpha1.FleetIngressAuthSpec{HubExposeLldapWebUI: true, HubLldapWebUIExposure: "wireguard"}, "wireguard"},
		{fleetv1alpha1.FleetIngressAuthSpec{HubLldapWebUIExposure: "authelia"}, "authelia"},
		{fleetv1alpha1.FleetIngressAuthSpec{HubLldapWebUIExposure: "Disabled"}, "disabled"},
		{fleetv1alpha1.FleetIngressAuthSpec{HubLldapWebUIExposure: "nonsense"}, "disabled"},
	}
	for _, c := range cases {
		if got := c.spec.EffectiveHubLldapWebUIExposure(); got != c.want {
			t.Fatalf("spec %+v -> %q, want %q", c.spec, got, c.want)
		}
	}
}

func TestDesiredLLDAPWebUIServiceByMode(t *testing.T) {
	cip := desiredLLDAPWebUIService("fleet-console", fleetv1alpha1.LldapWebUIExposureAuthelia, 30917)
	if cip.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("authelia mode service type = %s, want ClusterIP", cip.Spec.Type)
	}
	np := desiredLLDAPWebUIService("fleet-console", fleetv1alpha1.LldapWebUIExposureWireguard, 30917)
	if np.Spec.Type != corev1.ServiceTypeNodePort || np.Spec.Ports[0].NodePort != 30917 {
		t.Fatalf("wireguard mode service = %s/%d, want NodePort/30917", np.Spec.Type, np.Spec.Ports[0].NodePort)
	}
}

func TestWireguardExposureHasNoHubRoute(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = fleetv1alpha1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-traefik-dynamic", Namespace: "fleet-ingress"},
		Data:       map[string]string{"routes.yml": hubDynamicCMPlaceholder},
	}
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: "fleet-ingress"}}
	fia := &fleetv1alpha1.FleetIngressAuth{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-ingress-auth", Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetIngressAuthSpec{
			HubEnabled:            true,
			HubLldapWebUIExposure: fleetv1alpha1.LldapWebUIExposureWireguard,
			CookieDomain:          "demo.example",
		},
	}
	lldapDep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "lldap", Namespace: "fleet-console"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&fleetv1alpha1.FleetIngressAuth{}).WithObjects(cm, dep, fia, lldapDep).Build()
	cfg := config.Config{ConsoleNamespace: "fleet-console", HubGatewayNamespace: "fleet-ingress", HubGatewayDeployment: "gateway", HubDynamicCMName: "fleet-traefik-dynamic", LldapWebUINodePort: 30917}

	if err := ReconcileHubLLDAPWebUI(context.Background(), c, cfg, fia); err != nil {
		t.Fatal(err)
	}
	var got corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "fleet-ingress", Name: "fleet-traefik-dynamic"}, &got); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Data["routes.yml"], "fleet-lldap-webui") {
		t.Fatalf("wireguard mode must not publish a hub route:\n%s", got.Data["routes.yml"])
	}
	var svc corev1.Service
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "fleet-console", Name: "lldap-webui"}, &svc); err != nil {
		t.Fatal(err)
	}
	if svc.Spec.Type != corev1.ServiceTypeNodePort {
		t.Fatalf("wireguard mode service type = %s, want NodePort", svc.Spec.Type)
	}
	if fia.Status.Hub.LLDAPWebUIExposure != "wireguard" {
		t.Fatalf("status exposure = %q", fia.Status.Hub.LLDAPWebUIExposure)
	}
}
