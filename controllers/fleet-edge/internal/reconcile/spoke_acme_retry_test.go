package reconcile

import (
	"context"
	"fmt"
	"testing"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func spokeRoute(name, siteHost string, phase string) *fleetv1alpha1.FleetAppRoute {
	return &fleetv1alpha1.FleetAppRoute{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "fleet-system"},
		Spec: fleetv1alpha1.FleetAppRouteSpec{
			Cluster:  "c1",
			SiteHost: siteHost,
			Mode:     fleetv1alpha1.RouteModeSite,
		},
		Status: fleetv1alpha1.FleetAppRouteStatus{TLSPhase: phase},
	}
}

func spokeRetryReconciler(t *testing.T, objs ...client.Object) *RouteReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return &RouteReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
		Config: config.Config{ACMEEnabled: true},
	}
}

func spokeCluster() clusters.Cluster {
	return clusters.Cluster{Method: "port_forward", TlsPolicy: "acme_cluster"}
}

// A production issuance attempt that has gone stale must be re-stamped, because
// a one-shot token left a host that failed one challenge stuck in Issuing.
func TestSpokeProdACMERetryRefreshesStaleToken(t *testing.T) {
	stale := fmt.Sprintf("%d", time.Now().Add(-30*time.Minute).Unix())
	route := spokeRoute("app", "app.example.com", fleetv1alpha1.TLSPhaseIssuing)
	route.Annotations = map[string]string{annotationSpokeACMEProdRetry: stale}
	r := spokeRetryReconciler(t, route)

	got := r.ensureSpokeProdACMERetry(context.Background(), route, "c1", spokeCluster(), "app.example.com")
	if got == "" || got == stale {
		t.Fatalf("token = %q, want a refreshed token", got)
	}
	var latest fleetv1alpha1.FleetAppRoute
	if err := r.Get(context.Background(), client.ObjectKeyFromObject(route), &latest); err != nil {
		t.Fatal(err)
	}
	if latest.Annotations[annotationSpokeACMEProdRetry] != got {
		t.Fatalf("persisted token = %q, want %q", latest.Annotations[annotationSpokeACMEProdRetry], got)
	}
}

// A fresh token is returned untouched: refreshes are paced.
func TestSpokeProdACMERetryKeepsFreshToken(t *testing.T) {
	fresh := fmt.Sprintf("%d", time.Now().Unix())
	route := spokeRoute("app", "app.example.com", fleetv1alpha1.TLSPhaseIssuing)
	route.Annotations = map[string]string{annotationSpokeACMEProdRetry: fresh}
	r := spokeRetryReconciler(t, route)

	if got := r.ensureSpokeProdACMERetry(context.Background(), route, "c1", spokeCluster(), "app.example.com"); got != fresh {
		t.Fatalf("token = %q, want the existing %q", got, fresh)
	}
	if attempts := r.markProdRetryAttempt("c1", "app.example.com"); attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

// The escalation budget counts refreshes per host and resets once the
// certificate is live, so a later re-issuance starts a fresh episode.
func TestSpokeProdRetryAttemptsClearOnIssued(t *testing.T) {
	r := spokeRetryReconciler(t)
	if got := r.markProdRetryAttempt("c1", "app.example.com"); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
	if got := r.markProdRetryAttempt("c1", "app.example.com"); got != spokeACMEProdRetryRestartsAfter {
		t.Fatalf("attempts = %d, want %d (escalation threshold)", got, spokeACMEProdRetryRestartsAfter)
	}
	r.clearProdRetryAttempts("c1", "app.example.com")
	if got := r.markProdRetryAttempt("c1", "app.example.com"); got != 1 {
		t.Fatalf("attempts = %d after clear, want 1", got)
	}
}

// A host whose certificate is already live must not hold the cluster's ACME
// slot: the queued hostname has to become active.
func TestAdvanceSpokeACMEQueueReleasesIssuedHost(t *testing.T) {
	issued := spokeRoute("issued", "issued.example.com", fleetv1alpha1.TLSPhaseReady)
	waiting := spokeRoute("waiting", "waiting.example.com", fleetv1alpha1.TLSPhaseQueued)
	r := spokeRetryReconciler(t, issued, waiting)

	q := newSpokeACMEQueue()
	if !q.allow("c1", "issued.example.com") {
		t.Fatal("issued host should take the slot")
	}
	if q.allow("c1", "waiting.example.com") {
		t.Fatal("second host should wait")
	}
	r.spokeACME = q

	r.advanceSpokeACMEQueue(context.Background(), "c1", spokeCluster())

	if got := q.activeHost("c1"); got != "waiting.example.com" {
		t.Fatalf("active = %q, want waiting.example.com once the issued host released", got)
	}
}

// The spoke serves HTTPS on its NodePort, not 443: probing 443 made every queue
// check fail, so the queue could only advance by rotating on staleness.
func TestSpokeProbePortMatchesSpokeTraefik(t *testing.T) {
	c := spokeCluster()
	c.TraefikHTTPSPort = 30443
	if got := spokeProbeHTTPSPort(c); got != 30443 {
		t.Fatalf("probe port = %d, want 30443", got)
	}
	// Without a registered port the port_forward default still applies.
	if got := spokeProbeHTTPSPort(spokeCluster()); got != 30443 {
		t.Fatalf("default probe port = %d, want 30443", got)
	}
}
