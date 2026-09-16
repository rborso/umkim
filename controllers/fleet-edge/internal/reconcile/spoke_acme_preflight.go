package reconcile

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/dns"
)

const (
	certResolverProduction       = "letsencrypt"
	certResolverStaging          = "letsencrypt-staging"
	annotationSpokeACMEStagingOK = "edge.umkim.com/spoke-acme-staging-ok"
	acmeChallengePreflightPath   = "/.well-known/acme-challenge/preflight"
)

// spokeACMEState is the spoke/site ACME gate outcome for one route reconcile.
type spokeACMEState struct {
	ApplyACME    bool
	CertResolver string
	SkipApply    bool
	TLSPhase     string
	TLSMessage   string
	Blocked      bool
}

func spokeProbeHost(_ context.Context, hostname string, regCluster clusters.Cluster) string {
	if regCluster.EffectiveMethod() == "port_forward" {
		if host := strings.TrimSpace(regCluster.UpstreamHost); host != "" && !invalidProbeHost(host) {
			return host
		}
	}
	if !regCluster.DNSValidationRelaxed() {
		if ip := strings.TrimSpace(regCluster.EdgeWANIP); ip != "" && !invalidProbeHost(ip) {
			return ip
		}
	}
	_, ips, err := dns.ValidateHost(context.Background(), hostname, "")
	if err == nil && len(ips) > 0 {
		return ips[0]
	}
	if ip := strings.TrimSpace(regCluster.EdgeWANIP); ip != "" && !invalidProbeHost(ip) {
		return ip
	}
	if host := strings.TrimSpace(regCluster.UpstreamHost); host != "" {
		return host
	}
	return ""
}

func spokeProbeHTTPSPort(regCluster clusters.Cluster) int {
	if regCluster.EffectiveMethod() == "port_forward" {
		if p := regCluster.TraefikHTTPSPort; p > 0 {
			return p
		}
		return 30443
	}
	return 443
}

func spokeProbeHTTPPort(regCluster clusters.Cluster) int {
	if p := regCluster.TraefikHTTPPort; p > 0 {
		return p
	}
	return 30080
}

func validateSpokeConnectivity(ctx context.Context, hostname string) (bool, string) {
	return validateSpokeConnectivityForCluster(ctx, hostname, clusters.Cluster{})
}

func validateSpokeConnectivityForCluster(ctx context.Context, hostname string, regCluster clusters.Cluster) (bool, string) {
	hostname = normalizeHostname(hostname)
	if hostname == "" {
		return false, "empty site hostname"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if regCluster.EffectiveMethod() == "port_forward" {
		upstream := strings.TrimSpace(regCluster.UpstreamHost)
		if upstream != "" {
			httpPort := spokeProbeHTTPPort(regCluster)
			if msg := dialPublicPort(ctx, upstream, fmt.Sprintf("%d", httpPort)); msg == "" {
				if msg := probeHTTPChallengePathViaUpstream(ctx, hostname, upstream, httpPort); msg == "" {
					return true, ""
				}
				return false, msg
			}
		}
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupHost(lookupCtx, hostname)
	if err != nil || len(ips) == 0 {
		return false, fmt.Sprintf("DNS lookup failed for %s: %v", hostname, err)
	}
	for _, ip := range ips {
		if msg := dialPublicPort(ctx, ip, "80"); msg != "" {
			return false, msg
		}
		if msg := dialPublicPort(ctx, ip, "443"); msg != "" {
			return false, msg
		}
	}
	if msg := probeHTTPChallengePath(ctx, hostname); msg != "" {
		return false, msg
	}
	return true, ""
}

func dialPublicPort(ctx context.Context, ip, port string) string {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(dialCtx, "tcp", net.JoinHostPort(ip, port))
	if err != nil {
		return fmt.Sprintf("port %s unreachable on %s: %v", port, ip, err)
	}
	conn.Close()
	return ""
}

func probeHTTPChallengePath(ctx context.Context, hostname string) string {
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "http://"+hostname+acmeChallengePreflightPath, nil)
	if err != nil {
		return err.Error()
	}
	client := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL != nil && strings.Contains(req.URL.Path, "/.well-known/acme-challenge/") {
				if req.URL.Scheme == "https" || strings.HasPrefix(strings.ToLower(req.Header.Get("Location")), "https:") {
					return fmt.Errorf("HTTP-01 challenge path redirects to HTTPS")
				}
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "redirects to HTTPS") {
			return fmt.Sprintf("HTTP-01 challenge path on %s redirects to HTTPS (disable global HTTP→HTTPS redirect on spoke Traefik)", hostname)
		}
		return fmt.Sprintf("HTTP probe failed for %s: %v", hostname, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusPermanentRedirect ||
		resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "acme-challenge") && strings.HasPrefix(strings.ToLower(loc), "https:") {
			return fmt.Sprintf("HTTP-01 challenge path on %s redirects to HTTPS: %s", hostname, loc)
		}
	}
	return ""
}

func probeHTTPChallengePathViaUpstream(ctx context.Context, hostname, upstream string, port int) string {
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://%s:%d%s", upstream, port, acmeChallengePreflightPath)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return err.Error()
	}
	req.Host = hostname
	client := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL != nil && strings.Contains(req.URL.Path, "/.well-known/acme-challenge/") {
				if req.URL.Scheme == "https" || strings.HasPrefix(strings.ToLower(req.Header.Get("Location")), "https:") {
					return fmt.Errorf("HTTP-01 challenge path redirects to HTTPS")
				}
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "redirects to HTTPS") {
			return fmt.Sprintf("HTTP-01 challenge path on %s redirects to HTTPS (disable global HTTP→HTTPS redirect on spoke Traefik)", hostname)
		}
		return fmt.Sprintf("HTTP probe failed for %s via %s: %v", hostname, upstream, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusPermanentRedirect ||
		resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "acme-challenge") && strings.HasPrefix(strings.ToLower(loc), "https:") {
			return fmt.Sprintf("HTTP-01 challenge path on %s redirects to HTTPS: %s", hostname, loc)
		}
	}
	return ""
}
