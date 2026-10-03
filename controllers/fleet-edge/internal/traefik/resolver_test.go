package traefik

import (
	"strings"
	"testing"
)

func TestSetCertResolverForHostnames(t *testing.T) {
	yaml := []byte(`http:
  routers:
    fleet-app-a-tls:
      rule: Host(` + "`demo-hub.mgmt.umkim.com`" + `)
      tls:
        certResolver: letsencrypt
    fleet-app-b-tls:
      rule: Host(` + "`auth.mgmt.umkim.com`" + `)
      tls:
        certResolver: letsencrypt
`)
	out, err := SetCertResolverForHostnames(yaml, []string{"demo-hub.mgmt.umkim.com"}, CertResolverPreflight)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePreflightResolvers(out, []string{"demo-hub.mgmt.umkim.com"}); err != nil {
		t.Fatal(err)
	}
	hosts, _ := HostnamesForResolver(out, CertResolverProduction)
	if len(hosts) != 1 || hosts[0] != "auth.mgmt.umkim.com" {
		t.Fatalf("prod hosts %v", hosts)
	}
}

func TestEnsureACMETLSRoutersAddsPendingTLSWithMiddlewares(t *testing.T) {
	yaml := []byte(`http:
  routers:
    fleet-app-a-http:
      rule: Host(` + "`demo-hub.mgmt.umkim.com`" + `)
      service: fleet-app-a
      entryPoints:
        - web
  middlewares:
    fleet-edge-authelia:
      forwardAuth:
        address: http://authelia.example/api/authz/forward-auth
  services:
    fleet-app-a:
      loadBalancer:
        servers:
          - url: http://10.10.0.10:30080
`)
	mids := map[string][]string{
		"demo-hub.mgmt.umkim.com": {"fleet-edge-forward-headers-https", "fleet-edge-authelia"},
	}
	out, err := EnsureACMETLSRoutersWithMiddlewares(yaml, []string{"demo-hub.mgmt.umkim.com"}, CertResolverProduction, mids)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "fleet-acme-pending-demo-hub-mgmt-umkim-com-tls") {
		t.Fatalf("missing pending tls router: %s", s)
	}
	if !strings.Contains(s, "fleet-edge-authelia") {
		t.Fatalf("pending tls router should chain authelia: %s", s)
	}
}

func TestEnsureACMETLSRoutersAddsPendingTLS(t *testing.T) {
	yaml := []byte(`http:
  routers:
    fleet-app-a-http:
      rule: Host(` + "`demo-hub.mgmt.umkim.com`" + `)
      service: fleet-app-a
      entryPoints:
        - web
  services:
    fleet-app-a:
      loadBalancer:
        servers:
          - url: http://10.10.0.10:30080
`)
	out, err := EnsureACMETLSRouters(yaml, []string{"demo-hub.mgmt.umkim.com"}, CertResolverProduction)
	if err != nil {
		t.Fatal(err)
	}
	hosts, err := HostnamesForResolver(out, CertResolverProduction)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0] != "demo-hub.mgmt.umkim.com" {
		t.Fatalf("hosts %v", hosts)
	}
}
