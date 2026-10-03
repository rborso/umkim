package reconcile

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"
)

func waitACMECerts(ctx context.Context, probeHost string, hostnames []string, staging bool, timeout time.Duration) error {
	if probeHost == "" {
		probeHost = "127.0.0.1"
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		okCount := 0
		for _, host := range hostnames {
			ok, err := probeACMECert(probeHost, host, staging)
			if err != nil {
				lastErr = err
				continue
			}
			if ok {
				okCount++
			}
		}
		if okCount == len(hostnames) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("acme cert wait timed out after %s: %w", timeout, lastErr)
	}
	kind := "production"
	if staging {
		kind = "staging preflight"
	}
	return fmt.Errorf("acme %s cert wait timed out after %s for %v", kind, timeout, hostnames)
}

func probeACMECert(probeHost, hostname string, staging bool) (bool, error) {
	return probeACMECertOnPort(probeHost, 443, hostname, staging)
}

func probeACMECertOnPort(probeHost string, port int, hostname string, staging bool) (bool, error) {
	if port <= 0 {
		port = 443
	}
	addr := net.JoinHostPort(probeHost, fmt.Sprintf("%d", port))
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
		ServerName:         hostname,
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		return false, err
	}
	defer conn.Close()
	if len(conn.ConnectionState().PeerCertificates) == 0 {
		return false, fmt.Errorf("%s: no peer certificate", hostname)
	}
	issuer := conn.ConnectionState().PeerCertificates[0].Issuer.String()
	if staging {
		return strings.Contains(issuer, "(STAGING)") || strings.Contains(issuer, "Fake LE"), nil
	}
	if strings.Contains(issuer, "TRAEFIK DEFAULT CERT") {
		return false, nil
	}
	return strings.Contains(issuer, "Let's Encrypt") && !strings.Contains(issuer, "(STAGING)"), nil
}

func waitTLSListen(ctx context.Context, probeHost string, timeout time.Duration) error {
	if probeHost == "" {
		probeHost = "127.0.0.1"
	}
	addr := net.JoinHostPort(probeHost, "443")
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		dialer := &net.Dialer{Timeout: 3 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			conn.Close()
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("tls listen wait timed out after %s: %w", timeout, lastErr)
	}
	return fmt.Errorf("tls listen wait timed out after %s on %s", timeout, addr)
}
