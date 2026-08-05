package reconcile

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rborso/umkim/controllers/fleet-edge/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeACMEIssuer struct {
	mu       sync.Mutex
	order    []string
	fail     map[string]int
	attempts map[string]int
}

func (f *fakeACMEIssuer) fleetRoutesYAML(ctx context.Context) []byte {
	return []byte("http:\n  routers: {}\n")
}

func (f *fakeACMEIssuer) discoverPending(ctx context.Context, prevRoutesYAML, routesYAML []byte) []string {
	return nil
}

func (f *fakeACMEIssuer) validateDNS(ctx context.Context, hostname string) (bool, string) {
	return true, ""
}

func (f *fakeACMEIssuer) updateTLSPhase(ctx context.Context, hostname, phase, msg string) error {
	return nil
}

func (f *fakeACMEIssuer) onCertReady(ctx context.Context, hostname string) error {
	return nil
}

func (f *fakeACMEIssuer) issueHostname(ctx context.Context, hostname string, routesYAML []byte, skipStaging bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.order = append(f.order, hostname)
	if f.attempts == nil {
		f.attempts = map[string]int{}
	}
	f.attempts[hostname]++
	if n := f.fail[hostname]; n > 0 && f.attempts[hostname] <= n {
		return errors.New("simulated ACME failure")
	}
	return nil
}

func TestACMEQueueProcessesInOrderAndRotatesFailures(t *testing.T) {
	issuer := &fakeACMEIssuer{
		fail:     map[string]int{"a.example.com": 1},
		attempts: map[string]int{},
	}
	cfg := config.Config{
		ACMEEnabled:               true,
		ACMEQueueRetryIntervalSec: 1,
		ACMEDebounceSec:           0,
	}
	c := newACMECoordinator(cfg, issuer)
	c.debounce = 0
	c.enqueue("a.example.com", "b.example.com")

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		empty := len(c.queue) == 0 && !c.running
		c.mu.Unlock()
		if empty {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	issuer.mu.Lock()
	defer issuer.mu.Unlock()
	if len(issuer.order) < 3 {
		t.Fatalf("expected at least 3 issuance attempts, got %d (%v)", len(issuer.order), issuer.order)
	}
	if issuer.order[0] != "a.example.com" || issuer.order[1] != "b.example.com" {
		t.Fatalf("expected b to proceed while a failed first: %v", issuer.order)
	}
	if issuer.attempts["b.example.com"] != 1 {
		t.Fatalf("b should succeed once, got %d", issuer.attempts["b.example.com"])
	}
}

func TestGatewayRolloutCompleteRequiresReplicaCountMatch(t *testing.T) {
	three := int32(3)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Generation: 2},
		Spec:       appsv1.DeploymentSpec{Replicas: &three},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration:  2,
			Replicas:            4,
			UpdatedReplicas:     3,
			AvailableReplicas:   3,
			ReadyReplicas:       3,
		},
	}
	if gatewayRolloutComplete(dep, 3) {
		t.Fatal("expected incomplete rollout when status.replicas > desired")
	}
	dep.Status.Replicas = 3
	if !gatewayRolloutComplete(dep, 3) {
		t.Fatal("expected complete rollout when all replica counters match")
	}
}

func TestDiscoverPendingHostnamesMergesNewAndMissing(t *testing.T) {
	prev := []byte(`http:
  routers:
    fleet-authelia-portal-tls:
      rule: Host(` + "`auth.mgmt.umkim.com`" + `)
      tls:
        certResolver: letsencrypt
`)
	next := []byte(`http:
  routers:
    fleet-authelia-portal-tls:
      rule: Host(` + "`auth.mgmt.umkim.com`" + `)
      tls:
        certResolver: letsencrypt
    fleet-app-demo-tls:
      rule: Host(` + "`demo-hub.mgmt.umkim.com`" + `)
      tls:
        certResolver: letsencrypt
`)
	pending := discoverPendingHostnames(context.Background(), "", prev, next)
	if len(pending) != 2 {
		t.Fatalf("expected auth + demo pending without probe host, got %v", pending)
	}
}
