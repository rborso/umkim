package registry

import (
	"context"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSyncEdgeClustersFromSiteIngress(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = fleetv1alpha1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-edge-clusters", Namespace: "fleet-system"},
		Data: map[string]string{
			"clusters.yaml": `clusters:
  lab-single-0:
    site: homelab
    direct_ingress: true
    ingress_id: homelab-pf
    method: port_forward
    tls_policy: acme_cluster
    dns_zone: homelab.example.com
    edge_wan_ip: 203.0.113.99
    upstream_host: 10.10.0.12
    traefik_http_port: 30080
  other-site:
    site: remote
    direct_ingress: true
    ingress_id: remote-pf
    edge_wan_ip: 198.51.100.1
    upstream_host: 10.10.0.20
    traefik_http_port: 30080
`,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	fs := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab"},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName: "homelab",
			Edge: fleetv1alpha1.FleetSiteEdgeSpec{
				WanEgress: &fleetv1alpha1.FleetSiteWanEgressSpec{Enabled: true},
				Ingress: []fleetv1alpha1.FleetSiteIngressSpec{{
					ID: "homelab-pf", Method: fleetv1alpha1.IngressMethodPortForward,
					Zone: "homelab.example.com",
				}},
			},
		},
	}
	cfg := DomainConfig{EdgeClustersCMNamespace: "fleet-system", EdgeClustersCMName: "fleet-edge-clusters"}
	if err := SyncEdgeClustersFromSiteIngress(context.Background(), c, cfg, fs); err != nil {
		t.Fatal(err)
	}
	var updated corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "fleet-system", Name: "fleet-edge-clusters"}, &updated); err != nil {
		t.Fatal(err)
	}
	raw := updated.Data["clusters.yaml"]
	if !strings.Contains(raw, "wan_address_mode: dns") {
		t.Fatalf("expected wan_address_mode dns in:\n%s", raw)
	}
	if strings.Contains(raw, "edge_wan_ip: 203.0.113.99") {
		t.Fatalf("stale edge_wan_ip should be cleared:\n%s", raw)
	}
	if !strings.Contains(raw, "edge_wan_ip: 198.51.100.1") {
		t.Fatal("unrelated cluster should be preserved")
	}
}
