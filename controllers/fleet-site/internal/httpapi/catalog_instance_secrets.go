package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type catalogInstanceSecretsBody struct {
	Cluster    string            `json:"cluster"`
	Namespace  string            `json:"namespace"`
	SecretName string            `json:"secretName"`
	Data       map[string]string `json:"data"`
}

func (s *Server) handleCatalogInstanceSecrets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		s.putCatalogInstanceSecrets(r.Context(), w, r)
	case http.MethodDelete:
		s.deleteCatalogInstanceSecrets(r.Context(), w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) putCatalogInstanceSecrets(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	body, err := decodeCatalogInstanceSecretsBody(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.ensureSpokeCatalogSecret(ctx, body.Cluster, body.Namespace, body.SecretName, body.Data); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"cluster":    body.Cluster,
		"namespace":  body.Namespace,
		"secretName": body.SecretName,
		"phase":      "Ready",
	})
}

func (s *Server) deleteCatalogInstanceSecrets(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	body, err := decodeCatalogInstanceSecretsBody(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.deleteSpokeCatalogSecret(ctx, body.Cluster, body.Namespace, body.SecretName); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeCatalogInstanceSecretsBody(r *http.Request) (catalogInstanceSecretsBody, error) {
	var body catalogInstanceSecretsBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		return body, fmt.Errorf("invalid JSON body")
	}
	body.Cluster = strings.TrimSpace(body.Cluster)
	body.Namespace = strings.TrimSpace(body.Namespace)
	body.SecretName = strings.TrimSpace(body.SecretName)
	if body.Cluster == "" || body.Namespace == "" || body.SecretName == "" {
		return body, fmt.Errorf("cluster, namespace, and secretName are required")
	}
	if len(body.Data) == 0 {
		return body, fmt.Errorf("data must contain at least one key")
	}
	for k, v := range body.Data {
		if strings.TrimSpace(k) == "" || v == "" {
			return body, fmt.Errorf("data keys and values must be non-empty")
		}
	}
	return body, nil
}

func (s *Server) ensureSpokeCatalogSecret(ctx context.Context, cluster, namespace, name string, data map[string]string) error {
	cs, err := s.directSpokeClientset(ctx, cluster)
	if err != nil {
		return err
	}
	if err := ensureSpokeNamespace(ctx, cs, namespace); err != nil {
		return fmt.Errorf("ensure spoke namespace %s: %w", namespace, err)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "fleet-catalog",
				"fleet.umkim.com/cluster":      cluster,
			},
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: data,
	}
	_, err = cs.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create spoke secret %s/%s: %w", namespace, name, err)
	}
	existing, getErr := cs.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if getErr != nil {
		return fmt.Errorf("get spoke secret %s/%s: %w", namespace, name, getErr)
	}
	if existing.Data == nil {
		existing.Data = map[string][]byte{}
	}
	for k, v := range data {
		existing.Data[k] = []byte(v)
	}
	existing.StringData = nil
	_, err = cs.CoreV1().Secrets(namespace).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update spoke secret %s/%s: %w", namespace, name, err)
	}
	return nil
}

func (s *Server) deleteSpokeCatalogSecret(ctx context.Context, cluster, namespace, name string) error {
	cs, err := s.directSpokeClientset(ctx, cluster)
	if err != nil {
		return err
	}
	err = cs.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func ensureSpokeNamespace(ctx context.Context, cs kubernetes.Interface, namespace string) error {
	_, err := cs.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	_, err = cs.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "fleet-catalog",
			},
		},
	}, metav1.CreateOptions{})
	return err
}
