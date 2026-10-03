package llmkeys

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MasterKeyConfig names the LiteLLM master key secret (hub dcm namespace by default).
type MasterKeyConfig struct {
	DirectKey  string
	SecretNS   string
	SecretName string
	SecretKey  string
}

// LiteLLMMasterKey returns the LiteLLM upstream bearer token.
func LiteLLMMasterKey(ctx context.Context, k8s client.Client, cfg MasterKeyConfig) (string, error) {
	if v := strings.TrimSpace(cfg.DirectKey); v != "" {
		return v, nil
	}
	ns := strings.TrimSpace(cfg.SecretNS)
	name := strings.TrimSpace(cfg.SecretName)
	key := strings.TrimSpace(cfg.SecretKey)
	if ns == "" || name == "" {
		return "", nil
	}
	if key == "" {
		key = "master_key"
	}
	var sec corev1.Secret
	if err := k8s.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &sec); err != nil {
		return "", fmt.Errorf("read litellm master key secret %s/%s: %w", ns, name, err)
	}
	raw := sec.Data[key]
	if len(raw) == 0 {
		return "", fmt.Errorf("litellm master key secret %s/%s missing key %q", ns, name, key)
	}
	return string(raw), nil
}
