package registry

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSyncSiteIngressAssignmentsImplicitBinding(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = fleetv1alpha1.AddToScheme(scheme)

	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName: "homelab",
			SiteId:   "homelab",
			Edge: fleetv1alpha1.FleetSiteEdgeSpec{
				Ingress: []fleetv1alpha1.FleetSiteIngressSpec{
					{ID: "homelab-pf", Method: fleetv1alpha1.IngressMethodPortForward, Zone: "homelab.example.test"},
				},
			},
		},
	}
	// lab-implicit left ingress_id unset; the site has exactly one port_forward
	// entry, so it binds implicitly. lab-explicit names the entry outright.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-edge-clusters", Namespace: "fleet-system"},
		Data: map[string]string{
			"clusters.yaml": `clusters:
  lab-implicit:
    site: homelab
    method: port_forward
    upstream_host: 10.10.0.12
    traefik_http_port: 30080
  lab-explicit:
    site: homelab
    ingress_id: homelab-pf
    method: port_forward
    upstream_host: 10.10.0.13
    traefik_http_port: 30080
`,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(site, cm).Build()

	cfg := DomainConfig{
		FleetSiteNamespace:      "fleet-console",
		EdgeClustersCMNamespace: "fleet-system",
		EdgeClustersCMName:      "fleet-edge-clusters",
	}
	if err := SyncSiteIngressAssignments(context.Background(), c, cfg); err != nil {
		t.Fatal(err)
	}

	var got fleetv1alpha1.FleetSite
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "fleet-console", Name: "homelab"}, &got); err != nil {
		t.Fatal(err)
	}
	assigned := got.Spec.Edge.Ingress[0].AssignedClusters
	if len(assigned) != 2 || assigned[0] != "lab-explicit" || assigned[1] != "lab-implicit" {
		t.Fatalf("assignedClusters=%v want [lab-explicit lab-implicit]", assigned)
	}
}

func TestSyncSiteIngressAssignmentsIgnoresOtherSite(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = fleetv1alpha1.AddToScheme(scheme)

	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName: "homelab",
			SiteId:   "homelab",
			Edge: fleetv1alpha1.FleetSiteEdgeSpec{
				Ingress: []fleetv1alpha1.FleetSiteIngressSpec{
					{ID: "homelab-pf", Method: fleetv1alpha1.IngressMethodPortForward, Zone: "homelab.example.test"},
				},
			},
		},
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-edge-clusters", Namespace: "fleet-system"},
		Data: map[string]string{
			"clusters.yaml": `clusters:
  remote-cluster:
    site: remote
    ingress_id: homelab-pf
    method: port_forward
    upstream_host: 10.10.0.20
    traefik_http_port: 30080
`,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(site, cm).Build()
	cfg := DomainConfig{
		FleetSiteNamespace:      "fleet-console",
		EdgeClustersCMNamespace: "fleet-system",
		EdgeClustersCMName:      "fleet-edge-clusters",
	}
	if err := SyncSiteIngressAssignments(context.Background(), c, cfg); err != nil {
		t.Fatal(err)
	}
	var got fleetv1alpha1.FleetSite
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "fleet-console", Name: "homelab"}, &got); err != nil {
		t.Fatal(err)
	}
	if n := len(got.Spec.Edge.Ingress[0].AssignedClusters); n != 0 {
		t.Fatalf("assignedClusters=%v want empty for a cluster on another site", got.Spec.Edge.Ingress[0].AssignedClusters)
	}
}
