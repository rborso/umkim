package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/audit"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/config"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/identity"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/rbac"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/session"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDisabledUserLoginAndReenable(t *testing.T) {
	ns := "fleet-console"
	user := &authv1.FleetUser{
		ObjectMeta: metav1.ObjectMeta{Name: "ada", Namespace: ns},
		Spec: authv1.FleetUserSpec{
			ExternalID: "ada",
			Groups:     []string{"editors"},
			Disabled:   true,
		},
	}
	role := &authv1.FleetRole{
		ObjectMeta: metav1.ObjectMeta{Name: "dc-admin", Namespace: ns},
		Spec: authv1.FleetRoleSpec{Rules: []authv1.PolicyRule{{
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		}}},
	}
	binding := &authv1.FleetRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-binding", Namespace: ns},
		Spec: authv1.FleetRoleBindingSpec{
			RoleRef:  authv1.RoleRef{Name: "dc-admin"},
			Subjects: []authv1.SubjectRef{{Kind: "User", Name: "admin"}},
		},
	}
	scheme := runtime.NewScheme()
	if err := authv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, role, binding).Build()
	s := &Server{
		Client: c,
		Cfg: config.Config{
			Namespace:            ns,
			LoginRateLimitPerMin: 20,
			SessionTTL:           time.Hour,
		},
		Sessions: session.NewManager("test-secret", time.Hour, "fleet_session", false),
		Auth:     identity.NewAuthenticator(identity.NewHtpasswdEnvProvider("ada:secret", "")),
		RBAC:     &rbac.Engine{Client: c, Namespace: ns},
		Audit:    &audit.Recorder{Client: c, Namespace: ns},
	}

	login := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/login", bytes.NewReader([]byte(`{"username":"ada","password":"secret"}`)))
	loginRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(loginRec, login)
	if loginRec.Code != http.StatusForbidden {
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var stored authv1.FleetUser
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "ada"}, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Spec.Groups) != 1 || stored.Spec.Groups[0] != "editors" {
		t.Fatalf("groups refreshed: %#v", stored.Spec.Groups)
	}

	token, err := s.Sessions.Issue("ada", "ada", "htpasswd", "User", []string{"editors"})
	if err != nil {
		t.Fatal(err)
	}
	me := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/me", nil)
	me.AddCookie(&http.Cookie{Name: "fleet_session", Value: token})
	meRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(meRec, me)
	if meRec.Code != http.StatusUnauthorized {
		t.Fatalf("session status=%d body=%s", meRec.Code, meRec.Body.String())
	}

	adminToken, err := s.Sessions.Issue("admin", "admin", "htpasswd", "User", nil)
	if err != nil {
		t.Fatal(err)
	}
	patch := httptest.NewRequest(http.MethodPatch, "/api/v1alpha1/users/ada", bytes.NewReader([]byte(`{"spec":{"disabled":false}}`)))
	patch.AddCookie(&http.Cookie{Name: "fleet_session", Value: adminToken})
	patchRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(patchRec, patch)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patchRec.Code, patchRec.Body.String())
	}
	var patched authv1.FleetUser
	if err := json.Unmarshal(patchRec.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Spec.Disabled {
		t.Fatal("disabled flag still set")
	}

	login2 := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/login", bytes.NewReader([]byte(`{"username":"ada","password":"secret"}`)))
	login2Rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(login2Rec, login2)
	if login2Rec.Code != http.StatusOK {
		t.Fatalf("relogin status=%d body=%s", login2Rec.Code, login2Rec.Body.String())
	}
}
