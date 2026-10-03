package health

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ProbeHubPath checks reachability via the hub gateway with the app Host header.
func ProbeHubPath(ctx context.Context, client *http.Client, gatewayURL, host, path string) (bool, int, error) {
	if gatewayURL == "" || host == "" {
		return false, 0, fmt.Errorf("gateway URL and host required")
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := strings.TrimRight(gatewayURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, 0, err
	}
	req.Host = host
	req.Header.Set("Host", host)
	resp, err := client.Do(req)
	if err != nil {
		return false, 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	code := resp.StatusCode
	// Hub-only routes behind Authelia return 401/302 without a session; still reachable.
	// ACME hub routes redirect HTTP→HTTPS on the public hostname; do not follow (WAN :443 may be down).
	if code == http.StatusUnauthorized || code == http.StatusFound || code == http.StatusSeeOther ||
		code == http.StatusMovedPermanently || code == http.StatusPermanentRedirect {
		return true, code, nil
	}
	return code >= 200 && code < 400, code, nil
}

func NewHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // lab gateway probes
	return &http.Client{
		Timeout: timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
