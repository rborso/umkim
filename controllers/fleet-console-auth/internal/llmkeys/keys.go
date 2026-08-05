package llmkeys

import (
	"context"
	"strings"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidateBearer returns org slug when token matches an active FleetLLMApiKey secret.
func ValidateBearer(ctx context.Context, k8s client.Client, ns, token string) (KeyInfo, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return KeyInfo{}, false
	}
	var list authv1.FleetLLMApiKeyList
	if err := k8s.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return KeyInfo{}, false
	}
	for _, key := range list.Items {
		if !key.Status.Active {
			continue
		}
		secretName := key.Spec.SecretRef
		if secretName == "" {
			secretName = "fleet-llm-apikey-" + key.Name
		}
		var sec corev1.Secret
		if err := k8s.Get(ctx, client.ObjectKey{Namespace: ns, Name: secretName}, &sec); err != nil {
			continue
		}
		for _, v := range [][]byte{sec.Data["api_key"], sec.Data["token"], sec.Data["key"]} {
			if len(v) > 0 && string(v) == token {
				return KeyInfo{
					Org:    key.Spec.Org,
					Name:   key.Name,
					Models: append([]string(nil), key.Spec.Models...),
				}, true
			}
		}
	}
	return KeyInfo{}, false
}
