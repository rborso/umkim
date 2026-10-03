package traefik

import (
	"strings"
	"testing"
)

func TestBuildACMESolverRoutes(t *testing.T) {
	out, err := BuildACMESolverRoutes([]string{"app.example.com"}, CertResolverProduction)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "app.example.com") {
		t.Fatalf("missing host: %s", s)
	}
	if !strings.Contains(s, "certResolver: letsencrypt") {
		t.Fatalf("missing resolver: %s", s)
	}
}
