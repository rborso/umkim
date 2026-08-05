package llmkeys

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const tokenPrefix = "fleet_llm_"

type Manager struct {
	Client    client.Client
	Namespace string
}

type KeyInfo struct {
	Org    string
	Name   string
	Models []string
}

func (m *Manager) List(ctx context.Context, org string) ([]authv1.FleetLLMApiKey, error) {
	var list authv1.FleetLLMApiKeyList
	if err := m.Client.List(ctx, &list, client.InNamespace(m.Namespace)); err != nil {
		return nil, err
	}
	if org == "" {
		return list.Items, nil
	}
	out := make([]authv1.FleetLLMApiKey, 0, len(list.Items))
	for _, item := range list.Items {
		if item.Spec.Org == org {
			out = append(out, item)
		}
	}
	return out, nil
}

func (m *Manager) Get(ctx context.Context, org, name string) (*authv1.FleetLLMApiKey, error) {
	var key authv1.FleetLLMApiKey
	if err := m.Client.Get(ctx, types.NamespacedName{Namespace: m.Namespace, Name: name}, &key); err != nil {
		return nil, err
	}
	if org != "" && key.Spec.Org != org {
		return nil, apierrors.NewForbidden(authv1.GroupVersion.WithResource("fleetllmapikeys").GroupResource(), name, fmt.Errorf("org mismatch"))
	}
	return &key, nil
}

func (m *Manager) Create(ctx context.Context, org, name, displayName string, models []string) (string, *authv1.FleetLLMApiKey, error) {
	org = strings.TrimSpace(org)
	name = strings.TrimSpace(name)
	if org == "" || name == "" {
		return "", nil, fmt.Errorf("org and name required")
	}
	plain, err := randomToken()
	if err != nil {
		return "", nil, err
	}
	secretName := secretNameForKey(name)
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: m.Namespace,
			Labels: map[string]string{
				"fleet.umkim.com/llm-api-key":      "true",
				"fleet.umkim.com/org":              org,
				"fleet.umkim.com/llm-api-key-name": name,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{"api_key": []byte(plain)},
	}
	if err := m.Client.Create(ctx, sec); err != nil {
		return "", nil, err
	}
	key := &authv1.FleetLLMApiKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.Namespace,
			Labels: map[string]string{
				"fleet.umkim.com/org": org,
			},
		},
		Spec: authv1.FleetLLMApiKeySpec{
			Org:         org,
			DisplayName: displayName,
			SecretRef:   secretName,
			Models:      models,
		},
		Status: authv1.FleetLLMApiKeyStatus{
			KeyPrefix: prefixOf(plain),
			Active:    true,
		},
	}
	if err := m.Client.Create(ctx, key); err != nil {
		_ = m.Client.Delete(ctx, sec)
		return "", nil, err
	}
	key.Status = authv1.FleetLLMApiKeyStatus{
		KeyPrefix: prefixOf(plain),
		Active:    true,
	}
	if err := m.Client.Status().Update(ctx, key); err != nil {
		return "", nil, fmt.Errorf("update key status: %w", err)
	}
	return plain, key, nil
}

func (m *Manager) Delete(ctx context.Context, org, name string) error {
	key, err := m.Get(ctx, org, name)
	if err != nil {
		return err
	}
	secretName := key.Spec.SecretRef
	if secretName == "" {
		secretName = secretNameForKey(name)
	}
	var sec corev1.Secret
	if err := m.Client.Get(ctx, types.NamespacedName{Namespace: m.Namespace, Name: secretName}, &sec); err == nil {
		_ = m.Client.Delete(ctx, &sec)
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	return m.Client.Delete(ctx, key)
}

func (m *Manager) SetActive(ctx context.Context, org, name string, active bool) (*authv1.FleetLLMApiKey, error) {
	key, err := m.Get(ctx, org, name)
	if err != nil {
		return nil, err
	}
	key.Status.Active = active
	if err := m.Client.Status().Update(ctx, key); err != nil {
		return nil, fmt.Errorf("update key status: %w", err)
	}
	return key, nil
}

func (m *Manager) Rotate(ctx context.Context, org, name string) (string, *authv1.FleetLLMApiKey, error) {
	key, err := m.Get(ctx, org, name)
	if err != nil {
		return "", nil, err
	}
	plain, err := randomToken()
	if err != nil {
		return "", nil, err
	}
	secretName := key.Spec.SecretRef
	if secretName == "" {
		secretName = secretNameForKey(name)
	}
	var sec corev1.Secret
	if err := m.Client.Get(ctx, types.NamespacedName{Namespace: m.Namespace, Name: secretName}, &sec); err != nil {
		return "", nil, err
	}
	if sec.Data == nil {
		sec.Data = map[string][]byte{}
	}
	sec.Data["api_key"] = []byte(plain)
	if err := m.Client.Update(ctx, &sec); err != nil {
		return "", nil, err
	}
	key.Status.KeyPrefix = prefixOf(plain)
	key.Status.Active = true
	if err := m.Client.Status().Update(ctx, key); err != nil {
		return "", nil, fmt.Errorf("update key status: %w", err)
	}
	return plain, key, nil
}

func secretNameForKey(name string) string {
	return "fleet-llm-apikey-" + name
}

func prefixOf(token string) string {
	if len(token) <= 12 {
		return token
	}
	return token[:12] + "…"
}

func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return tokenPrefix + hex.EncodeToString(b), nil
}

func ModelAllowed(models []string, model string) bool {
	if len(models) == 0 {
		return true
	}
	model = strings.TrimSpace(model)
	for _, allowed := range models {
		if strings.TrimSpace(allowed) == model {
			return true
		}
	}
	return false
}
