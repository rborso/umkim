package ingressauth

import (
	"context"
	"strings"
	"testing"

	fleetedge "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestComputeDesiredScopesHubAccessDomains verifies the hub Authelia only allows
// the hub zone: a site-routed zone belongs to the spoke's Authelia, not the hub.
func TestComputeDesiredScopesHubAccessDomains(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := fleetedge.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName: "homelab",
			SiteId:   "homelab",
			Edge: fleetv1alpha1.FleetSiteEdgeSpec{
				Ingress: []fleetv1alpha1.FleetSiteIngressSpec{
					{ID: "homelab-pf", Method: fleetv1alpha1.IngressMethodPortForward, Zone: "homelab.example.test", Address: "172.16.10.100"},
				},
			},
		},
	}
	join := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "lab-single-0", Namespace: ns},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName: "lab-single-0",
			SiteId:      "homelab",
			Edge:        fleetv1alpha1.FleetClusterJoinEdgeSpec{IngressId: "homelab-pf"},
			IngressAuth: fleetv1alpha1.FleetClusterJoinIngressAuthSpec{Enabled: true},
		},
		Status: fleetv1alpha1.FleetClusterJoinStatus{Phase: fleetv1alpha1.JoinPhaseReady},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: ClusterSecretName("lab-single-0"), Namespace: ns},
	}
	fia := &fleetv1alpha1.FleetIngressAuth{
		ObjectMeta: metav1.ObjectMeta{Name: fleetv1alpha1.FleetIngressAuthName, Namespace: ns},
		Spec: fleetv1alpha1.FleetIngressAuthSpec{
			PortalHost:    "auth.demo.example.test",
			CookieDomain:  "demo.example.test",
			AccessDomains: []string{"*.demo.example.test", "demo.example.test"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(site, join, secret).Build()

	desired, err := ComputeDesired(context.Background(), c, config.Config{Namespace: ns, HubEdgeDomain: "demo.example.test"}, fia)
	if err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(desired.AccessDomains, ",")
	if strings.Contains(joined, "homelab.example.test") {
		t.Fatalf("hub access domains must not include a site zone: %v", desired.AccessDomains)
	}
	if !strings.Contains(joined, "demo.example.test") {
		t.Fatalf("hub access domains must include the hub zone: %v", desired.AccessDomains)
	}
	cc, ok := desired.ClusterConfigs["lab-single-0"]
	if !ok {
		t.Fatalf("cluster config missing: %+v", desired.ClusterConfigs)
	}
	if cc.CookieDomain != "homelab.example.test" {
		t.Fatalf("spoke cookie domain = %q want homelab.example.test", cc.CookieDomain)
	}
	if strings.Join(cc.AccessDomains, ",") != "*.homelab.example.test,homelab.example.test" {
		t.Fatalf("spoke access domains = %v", cc.AccessDomains)
	}
}
