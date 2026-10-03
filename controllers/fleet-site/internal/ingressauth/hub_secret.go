package ingressauth

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// EnsureHubSecretFromInventory seeds or updates the hub ingress-auth Secret when inventory
// supplies a bootstrap password (ansible-vault on the hub).
func EnsureHubSecretFromInventory(ctx context.Context, c client.Client, ns, secretName, bootstrapPassword string) error {
	if bootstrapPassword == "" {
		return nil
	}
	if len(bootstrapPassword) < MinLLDAPAdminPasswordLen {
		return fmt.Errorf("bootstrap password must be at least %d characters", MinLLDAPAdminPasswordLen)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name": "fleet-ingress-auth",
			},
		},
		Type: corev1.SecretTypeOpaque,
	}
	_, err := controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
		if secret.StringData == nil {
			secret.StringData = map[string]string{}
		}
		secret.StringData["bootstrapPassword"] = bootstrapPassword
		existing := secret.Data
		if existing == nil {
			existing = map[string][]byte{}
		}
		for _, key := range []string{"lldapJwtSecret", "sessionSecret", "storageEncryptionKey", "resetJwtSecret"} {
			if len(existing[key]) == 0 {
				secret.StringData[key] = randToken(32)
			}
		}
		return nil
	})
	return err
}

// HubSecretReady reports whether the hub ingress-auth Secret exists with a bootstrap password.
func HubSecretReady(ctx context.Context, c client.Client, ns, secretName string) bool {
	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: secretName}, &secret); err != nil {
		return false
	}
	return len(secret.Data["bootstrapPassword"]) > 0
}
