package reconcile

import (
	"testing"
	"time"

	"github.com/rborso/umkim/controllers/fleet-edge/internal/traefik"
	appsv1 "k8s.io/api/apps/v1"
)

func TestGatewayACMEWaitDuration(t *testing.T) {
	if got := gatewayACMEWaitDuration(gatewayACMEPreflightWait, 1); got != 90*time.Second {
		t.Fatalf("preflight single: got %s", got)
	}
	if got := gatewayACMEWaitDuration(gatewayACMEProductionWait, 3); got != 390*time.Second {
		t.Fatalf("production three: got %s", got)
	}
	if got := gatewayACMEWaitDuration(gatewayACMEProductionWait, 20); got != gatewayACMECertObtainWaitMax {
		t.Fatalf("capped: got %s", got)
	}
}

func TestNewHostnamesDetectsAddedHubRoutes(t *testing.T) {
	prev := []byte(`http:
  routers:
    fleet-authelia-portal-tls:
      rule: Host(` + "`auth.mgmt.umkim.com`" + `)
`)
	next := []byte(`http:
  routers:
    fleet-authelia-portal-tls:
      rule: Host(` + "`auth.mgmt.umkim.com`" + `)
    fleet-app-demo-tls:
      rule: Host(` + "`demo-hub.mgmt.umkim.com`" + `)
      tls:
        certResolver: letsencrypt
`)
	added := traefik.NewHostnames(prev, next)
	if len(added) != 1 || added[0] != "demo-hub.mgmt.umkim.com" {
		t.Fatalf("got %v", added)
	}
	if len(traefik.NewHostnames(next, next)) != 0 {
		t.Fatal("identical YAML must not report new hostnames")
	}
}

func TestGatewayNeedsACMEMode(t *testing.T) {
	three := int32(3)
	dep := appsv1.Deployment{}
	dep.Spec.Replicas = &three
	if !gatewayNeedsACMEMode(dep) {
		t.Fatal("expected HA gateway to need ACME mode")
	}

	one := int32(1)
	dep.Spec.Replicas = &one
	dep.Spec.Template.Spec.HostNetwork = true
	if gatewayNeedsACMEMode(dep) {
		t.Fatal("single hostNetwork gateway should not need ACME mode")
	}
}
