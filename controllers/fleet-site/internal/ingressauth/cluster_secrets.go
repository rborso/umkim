package ingressauth

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	MinLLDAPAdminPasswordLen = 12
	clusterSecretPrefix        = "fleet-ingress-auth-"
)

func ClusterSecretName(clusterName string) string {
	return clusterSecretPrefix + clusterName
}

// EnsureClusterSecret writes lldapAdminPassword and preserves generated Authelia keys on re-enable.
func EnsureClusterSecret(ctx context.Context, c client.Client, ns, clusterName, lldapAdminPassword string) (string, error) {
	if len(lldapAdminPassword) < MinLLDAPAdminPasswordLen {
		return "", fmt.Errorf("lldapAdminPassword must be at least %d characters", MinLLDAPAdminPasswordLen)
	}
	name := ClusterSecretName(clusterName)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "fleet-ingress-auth",
				"fleet.umkim.com/cluster":     clusterName,
			},
		},
		Type: corev1.SecretTypeOpaque,
	}
	_, err := controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
		if secret.StringData == nil {
			secret.StringData = map[string]string{}
		}
		secret.StringData["lldapAdminPassword"] = lldapAdminPassword
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
	if err != nil {
		return "", err
	}
	return name, nil
}

func ClusterSecretExists(ctx context.Context, c client.Client, ns, clusterName string) bool {
	var secret corev1.Secret
	err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: ClusterSecretName(clusterName)}, &secret)
	return err == nil
}

func DeleteClusterSecret(ctx context.Context, c client.Client, ns, clusterName string) error {
	var secret corev1.Secret
	name := ClusterSecretName(clusterName)
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return c.Delete(ctx, &secret)
}
