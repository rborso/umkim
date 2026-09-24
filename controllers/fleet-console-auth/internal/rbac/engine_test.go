package rbac

import (
	"context"
	"testing"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAuthorizeScopedBinding(t *testing.T) {
	ns := "fleet-console"
	scheme := runtime.NewScheme()
	if err := authv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	siteAdmin := &authv1.FleetRole{
		ObjectMeta: metav1.ObjectMeta{Name: "site-admin", Namespace: ns},
		Spec: authv1.FleetRoleSpec{Rules: []authv1.PolicyRule{
			{Resources: []string{"sites", "clusters", "apps", "charts"}, Verbs: []string{"get", "list", "create", "update", "delete", "patch"}},
		}},
	}
	dcAdmin := &authv1.FleetRole{
		ObjectMeta: metav1.ObjectMeta{Name: "dc-admin", Namespace: ns},
		Spec:       authv1.FleetRoleSpec{Rules: []authv1.PolicyRule{{Resources: []string{"*"}, Verbs: []string{"*"}}}},
	}
	siteBinding := &authv1.FleetRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "ada-site", Namespace: ns},
		Spec: authv1.FleetRoleBindingSpec{
			Subjects: []authv1.SubjectRef{{Kind: "User", Name: "ada"}},
			RoleRef:  authv1.RoleRef{Name: "site-admin"},
			Scopes:   []authv1.ScopeRef{{Kind: "Site", Name: "homelab"}},
		},
	}
	wildBinding := &authv1.FleetRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "bea-admin", Namespace: ns},
		Spec: authv1.FleetRoleBindingSpec{
			Subjects: []authv1.SubjectRef{{Kind: "User", Name: "bea"}},
			RoleRef:  authv1.RoleRef{Name: "dc-admin"},
			Scopes:   []authv1.ScopeRef{{Kind: "*", Name: "*"}},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(siteAdmin, dcAdmin, siteBinding, wildBinding).Build()
	e := &Engine{Client: c, Namespace: ns}
	ada := Actor{Kind: "User", Name: "ada"}
	bea := Actor{Kind: "User", Name: "bea"}

	cases := []struct {
		name  string
		actor Actor
		req   Request
		allow bool
	}{
		{"site admin delete app", ada, Request{Resource: "apps", Verb: "delete"}, false},
		{"site admin create chart", ada, Request{Resource: "charts", Verb: "create"}, false},
		{"site admin list apps", ada, Request{Resource: "apps", Verb: "list"}, true},
		{"site admin patch own site", ada, Request{Resource: "sites", Verb: "patch", ScopeKind: "Site", ScopeName: "homelab"}, true},
		{"site admin patch other site", ada, Request{Resource: "sites", Verb: "patch", ScopeKind: "Site", ScopeName: "other"}, false},
		{"wildcard create app", bea, Request{Resource: "apps", Verb: "create"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := e.Authorize(context.Background(), tc.actor, tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if ok != tc.allow {
				t.Fatalf("allow=%v want %v", ok, tc.allow)
			}
		})
	}
}
