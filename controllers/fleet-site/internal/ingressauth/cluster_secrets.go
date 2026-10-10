package ingressauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"

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
		for _, key := range []string{"lldapJwtSecret", "sessionSecret", "storageEncryptionKey", "resetJwtSecret", "oidcHmacSecret"} {
			if len(existing[key]) == 0 {
				secret.StringData[key] = randToken(32)
			}
		}
		if len(existing["oidcIssuerPrivateKey"]) == 0 {
			key, err := rsaPrivateKeyPEM()
			if err != nil {
				return err
			}
			secret.StringData["oidcIssuerPrivateKey"] = key
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return name, nil
}

// EnsureOIDCSecrets generates and persists the Authelia OIDC provider keys
// (hmac secret, issuer private key) and a secret per declared OIDC client id,
// returning the client-id -> secret map. Idempotent.
func EnsureOIDCSecrets(ctx context.Context, c client.Client, ns, clusterName string, clientIDs []string) (map[string]string, error) {
	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: ClusterSecretName(clusterName)}, &secret); err != nil {
		return nil, err
	}
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	changed := false
	if len(secret.Data["oidcHmacSecret"]) == 0 {
		secret.Data["oidcHmacSecret"] = []byte(randToken(32))
		changed = true
	}
	if len(secret.Data["oidcIssuerPrivateKey"]) == 0 {
		key, err := rsaPrivateKeyPEM()
		if err != nil {
			return nil, err
		}
		secret.Data["oidcIssuerPrivateKey"] = []byte(key)
		changed = true
	}
	existing := map[string]string{}
	if raw := secret.Data["oidcClientSecrets"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &existing)
	}
	clientChanged := false
	for _, id := range clientIDs {
		id = strings.TrimSpace(id)
		if id == "" || existing[id] != "" {
			continue
		}
		existing[id] = randToken(32)
		clientChanged = true
	}
	if clientChanged {
		b, err := json.Marshal(existing)
		if err != nil {
			return nil, err
		}
		secret.Data["oidcClientSecrets"] = b
		changed = true
	}
	if changed {
		if err := c.Update(ctx, &secret); err != nil {
			return nil, err
		}
	}
	return existing, nil
}

// rsaPrivateKeyPEM returns a PKCS#8 RSA private key in PEM (Authelia OIDC issuer key).
func rsaPrivateKeyPEM() (string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
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
