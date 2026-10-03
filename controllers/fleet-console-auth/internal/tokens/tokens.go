package tokens

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const tokenPrefix = "fleet_sa_"

type Manager struct {
	Client    client.Client
	Namespace string
}

func (m *Manager) Validate(ctx context.Context, raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("missing token")
	}
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])
	var secrets corev1.SecretList
	if err := m.Client.List(ctx, &secrets, client.InNamespace(m.Namespace), client.MatchingLabels{
		"fleet.umkim.com/service-account-token": "true",
	}); err != nil {
		return "", err
	}
	for _, sec := range secrets.Items {
		if string(sec.Data["tokenHash"]) == hash {
			return sec.Labels["fleet.umkim.com/service-account"], nil
		}
	}
	return "", fmt.Errorf("invalid token")
}

func (m *Manager) Issue(ctx context.Context, saName string) (string, error) {
	plain, err := randomToken()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(plain))
	hash := hex.EncodeToString(sum[:])
	secretName := fmt.Sprintf("fleet-sa-%s-token", saName)
	sec := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: m.Namespace,
			Labels: map[string]string{
				"fleet.umkim.com/service-account-token": "true",
				"fleet.umkim.com/service-account":       saName,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"tokenHash": []byte(hash),
		},
	}
	var existing corev1.Secret
	err = m.Client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: m.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		if err := m.Client.Create(ctx, &sec); err != nil {
			return "", err
		}
	} else if err == nil {
		existing.Data = sec.Data
		if err := m.Client.Update(ctx, &existing); err != nil {
			return "", err
		}
	} else {
		return "", err
	}
	var sa authv1.FleetServiceAccount
	if err := m.Client.Get(ctx, types.NamespacedName{Name: saName, Namespace: m.Namespace}, &sa); err == nil {
		sa.Status.TokenSecretName = secretName
		_ = m.Client.Status().Update(ctx, &sa)
	}
	return plain, nil
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return tokenPrefix + hex.EncodeToString(b), nil
}
