package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"k8s.io/client-go/rest"
)

func (s *Server) vmSubresource(ctx context.Context, cfg *rest.Config, ns, name, action string, force bool) error {
	path := fmt.Sprintf("/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachines/%s/%s", ns, name, action)
	var body io.Reader
	contentType := ""
	if action == "stop" && force {
		body = bytes.NewReader([]byte(`{"gracePeriod":0}`))
		contentType = "application/json"
	}
	if action == "restart" && force {
		body = bytes.NewReader([]byte(`{"gracePeriodSeconds":0}`))
		contentType = "application/json"
	}
	resp, err := spokeDo(ctx, cfg, http.MethodPut, path, body, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(b))
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("kubevirt %s: %s", action, msg)
}

func spokeDo(ctx context.Context, cfg *rest.Config, method, path string, body io.Reader, contentType string) (*http.Response, error) {
	rt, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, err
	}
	host := strings.TrimSuffix(cfg.Host, "/")
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	req, err := http.NewRequestWithContext(ctx, method, host+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return rt.RoundTrip(req)
}
