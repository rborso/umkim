package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBootstrapScriptRequiresIdentity(t *testing.T) {
	ns := "fleet-console"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: bootstrapSecretName("homelab"), Namespace: ns},
		Data:       map[string][]byte{"bootstrap.sh": []byte("#!/bin/sh\necho ok\n")},
	}
	s := &Server{
		Client: fake.NewClientBuilder().WithScheme(testSiteHTTPScheme(t)).WithObjects(secret).Build(),
		Cfg:    config.Config{Namespace: ns, RequireIdentity: true},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/sites/homelab/bootstrap-script", nil)
	rec := httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1alpha1/sites/homelab/bootstrap-script", nil)
	req.Header.Set(headerSubjectKind, "User")
	req.Header.Set(headerSubjectName, "ada")
	rec = httptest.NewRecorder()
	s.handleSiteByName(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "echo ok") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}
