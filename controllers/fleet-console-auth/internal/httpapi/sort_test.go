package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/audit"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/config"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/rbac"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSortByName(t *testing.T) {
	items := []metav1.PartialObjectMetadata{
		{ObjectMeta: metav1.ObjectMeta{Name: "zoe"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "ada"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "mallory"}},
	}
	sortByName(items, func(o metav1.PartialObjectMetadata) string { return o.Name })
	want := []string{"ada", "mallory", "zoe"}
	for i := range want {
		if items[i].Name != want[i] {
			t.Fatalf("item %d = %q want %q", i, items[i].Name, want[i])
		}
	}
}

func TestListUsersIsSorted(t *testing.T) {
	ns := "fleet-console"
	scheme := runtime.NewScheme()
	if err := authv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	objs := []client.Object{
		&authv1.FleetUser{ObjectMeta: metav1.ObjectMeta{Name: "zoe", Namespace: ns}},
		&authv1.FleetUser{ObjectMeta: metav1.ObjectMeta{Name: "ada", Namespace: ns}},
		&authv1.FleetUser{ObjectMeta: metav1.ObjectMeta{Name: "mallory", Namespace: ns}},
		&authv1.FleetRole{ObjectMeta: metav1.ObjectMeta{Name: "dc-admin", Namespace: ns}, Spec: authv1.FleetRoleSpec{
			Rules: []authv1.PolicyRule{{Resources: []string{"*"}, Verbs: []string{"*"}}},
		}},
		&authv1.FleetRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "admin-binding", Namespace: ns}, Spec: authv1.FleetRoleBindingSpec{
			RoleRef:  authv1.RoleRef{Name: "dc-admin"},
			Subjects: []authv1.SubjectRef{{Kind: "Group", Name: "dc-admins"}},
		}},
	}
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	s := &Server{
		Client: k8s,
		// AuthBypass resolves the actor to a dc-admins group member, so the
		// handler runs without a session cookie.
		Cfg:   config.Config{Namespace: ns, AuthBypass: true},
		RBAC:  &rbac.Engine{Client: k8s, Namespace: ns},
		Audit: &audit.Recorder{Client: k8s, Namespace: ns},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	s.handleUsers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Users []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(body.Users))
	for _, u := range body.Users {
		got = append(got, u.Metadata.Name)
	}
	if strings.Join(got, ",") != "ada,mallory,zoe" {
		t.Fatalf("users order=%v want [ada mallory zoe]", got)
	}
}
