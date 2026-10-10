package reconcile

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileSpokeACMESkipDoesNotMarkApplied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}

	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	const ns = "fleet-system"
	clustersCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-edge-clusters", Namespace: ns},
		Data: map[string]string{
			"clusters.yaml": fmt.Sprintf(`clusters:
  lab:
    method: port_forward
    tls_policy: acme_cluster
    direct_ingress: true
    upstream_host: 127.0.0.1
    traefik_http_port: %d
`, port),
		},
	}
	route := &fleetv1alpha1.FleetAppRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "app",
			Namespace:  ns,
			Finalizers: []string{finalizerName},
		},
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID: "app",
			Mode:       fleetv1alpha1.RouteModeSite,
			Cluster:    "lab",
			HubHost:    "app.example.com",
			SiteHost:   "app.lab.example.com",
			Service:    fleetv1alpha1.ServiceRef{Namespace: "default", Name: "app", Port: 80},
		},
	}
	// A legitimate holder for the cluster's ACME slot: it has a real route, so it
	// is not dropped by the route-less cleanup.
	holder := &fleetv1alpha1.FleetAppRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "holder",
			Namespace:  ns,
			Finalizers: []string{finalizerName},
		},
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			InstanceID: "holder",
			Mode:       fleetv1alpha1.RouteModeSite,
			Cluster:    "lab",
			HubHost:    "other.example.com",
			SiteHost:   "other.example.com",
			Service:    fleetv1alpha1.ServiceRef{Namespace: "default", Name: "holder", Port: 80},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(route).WithObjects(clustersCM, route, holder).Build()
	queue := newSpokeACMEQueue()
	queue.active["lab"] = "other.example.com"
	r := &RouteReconciler{
		Client: c,
		Config: config.Config{
			ClusterConfigMapNS:    ns,
			ClusterConfigMapName:  "fleet-edge-clusters",
			FleetRoutesCMNS:       "fleet-ingress",
			FleetRoutesCMName:     "fleet-traefik-fleet-routes",
			FleetRoutesCMKey:      "fleet-routes.yml",
			FleetEdgeDomain:       "example.com",
			ACMEEnabled:           true,
			ACMEDNSCheckEnabled:   false,
			RequeueSeconds:        20,
			GatewayDeploymentNS:   "fleet-ingress",
			GatewayDeploymentName: "gateway",
		},
		spokeACME: queue,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(route)})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 20*time.Second {
		t.Fatalf("requeue = %s, want 20s", result.RequeueAfter)
	}

	var got fleetv1alpha1.FleetAppRoute
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(route), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase == fleetv1alpha1.PhaseApplied {
		t.Fatalf("phase = %q, want held", got.Status.Phase)
	}
	if got.Status.TLSPhase != fleetv1alpha1.TLSPhaseQueued {
		t.Fatalf("tls phase = %q, want Queued", got.Status.TLSPhase)
	}
	if got.Status.Message == "" {
		t.Fatal("expected route message from the skipped ACME state")
	}
}
