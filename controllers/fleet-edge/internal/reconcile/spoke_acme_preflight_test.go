package reconcile

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
)

func TestDialPublicPortInvalidIP(t *testing.T) {
	msg := dialPublicPort(t.Context(), "203.0.113.199", "59999")
	if msg == "" {
		t.Fatal("expected dial failure for closed port")
	}
}

func TestProbeHTTPChallengePathEmptyHost(t *testing.T) {
	ok, msg := validateSpokeConnectivity(t.Context(), "")
	if ok || msg == "" {
		t.Fatalf("expected failure for empty host, ok=%v msg=%q", ok, msg)
	}
}

func TestPortForwardTLSRedirectDoesNotDropProbeMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://app.example.com"+r.URL.Path, http.StatusMovedPermanently)
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
	cluster := clusters.Cluster{
		Method:           "port_forward",
		UpstreamHost:     "127.0.0.1",
		TraefikHTTPPort:  port,
		TraefikHTTPSPort: 1,
	}
	ok, msg := validateSpokeConnectivityForCluster(t.Context(), "app.example.com", cluster)
	if ok || msg == "" {
		t.Fatalf("closed https port after HTTP redirect: ok=%v msg=%q", ok, msg)
	}

	cluster.TraefikHTTPSPort = port
	ok, msg = validateSpokeConnectivityForCluster(t.Context(), "app.example.com", cluster)
	if !ok || msg != "" {
		t.Fatalf("TLS-ALPN redirect with https port open: ok=%v msg=%q", ok, msg)
	}
}
