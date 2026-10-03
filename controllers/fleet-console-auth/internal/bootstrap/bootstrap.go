package bootstrap

import (
	"context"
	_ "embed"
	"strings"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrl "sigs.k8s.io/controller-runtime/pkg/log"
)

//go:embed seed-roles.yaml
var seedRolesYAML string

type Config struct {
	Namespace           string
	BootstrapAdminUser  string
	BootstrapLDAPGroup  string
	BootstrapLDAPGroupCN string
}

func Run(ctx context.Context, c client.Client, cfg Config) error {
	log := ctrl.FromContext(ctx).WithName("bootstrap")
	if err := ensureSeedRoles(ctx, c, cfg.Namespace); err != nil {
		return err
	}
	if cfg.BootstrapAdminUser != "" {
		if err := ensureAdminUserBinding(ctx, c, cfg.Namespace, cfg.BootstrapAdminUser); err != nil {
			return err
		}
		log.Info("ensured bootstrap admin binding", "user", cfg.BootstrapAdminUser)
	}
	if cfg.BootstrapLDAPGroup != "" && cfg.BootstrapLDAPGroupCN != "" {
		if err := ensureLDAPAdminGroup(ctx, c, cfg.Namespace, cfg.BootstrapLDAPGroup, cfg.BootstrapLDAPGroupCN); err != nil {
			return err
		}
		log.Info("ensured lldap admin group binding", "group", cfg.BootstrapLDAPGroup)
	}
	return nil
}

func ensureSeedRoles(ctx context.Context, c client.Client, ns string) error {
	docs := strings.Split(seedRolesYAML, "---")
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		var role authv1.FleetRole
		if err := yaml.Unmarshal([]byte(doc), &role); err != nil {
			return err
		}
		role.Namespace = ns
		var existing authv1.FleetRole
		err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: role.Name}, &existing)
		if apierrors.IsNotFound(err) {
			if err := c.Create(ctx, &role); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		existing.Spec = role.Spec
		if err := c.Update(ctx, &existing); err != nil {
			return err
		}
	}
	return nil
}

func ensureAdminUserBinding(ctx context.Context, c client.Client, ns, username string) error {
	rb := authv1.FleetRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-admin-user", Namespace: ns},
		Spec: authv1.FleetRoleBindingSpec{
			Subjects: []authv1.SubjectRef{{Kind: "User", Name: username}},
			RoleRef:  authv1.RoleRef{Name: "dc-admin"},
			Scopes:   []authv1.ScopeRef{{Kind: "*", Name: "*"}},
		},
	}
	var existing authv1.FleetRoleBinding
	err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: rb.Name}, &existing)
	if apierrors.IsNotFound(err) {
		return c.Create(ctx, &rb)
	}
	if err != nil {
		return err
	}
	existing.Spec = rb.Spec
	return c.Update(ctx, &existing)
}

func ensureLDAPAdminGroup(ctx context.Context, c client.Client, ns, groupName, ldapCN string) error {
	g := authv1.FleetGroup{
		ObjectMeta: metav1.ObjectMeta{Name: groupName, Namespace: ns},
		Spec: authv1.FleetGroupSpec{
			DisplayName: "LLDAP administrators",
			LDAPGroupCN: ldapCN,
		},
	}
	var existingG authv1.FleetGroup
	err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: groupName}, &existingG)
	if apierrors.IsNotFound(err) {
		if err := c.Create(ctx, &g); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if existingG.Spec.LDAPGroupCN != ldapCN || existingG.Spec.DisplayName != g.Spec.DisplayName {
		existingG.Spec.LDAPGroupCN = ldapCN
		existingG.Spec.DisplayName = g.Spec.DisplayName
		if err := c.Update(ctx, &existingG); err != nil {
			return err
		}
	}

	rb := authv1.FleetRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-lldap-admin-group", Namespace: ns},
		Spec: authv1.FleetRoleBindingSpec{
			Subjects: []authv1.SubjectRef{{Kind: "Group", Name: groupName}},
			RoleRef:  authv1.RoleRef{Name: "dc-admin"},
			Scopes:   []authv1.ScopeRef{{Kind: "*", Name: "*"}},
		},
	}
	var existingRB authv1.FleetRoleBinding
	err = c.Get(ctx, types.NamespacedName{Namespace: ns, Name: rb.Name}, &existingRB)
	if apierrors.IsNotFound(err) {
		return c.Create(ctx, &rb)
	}
	return err
}

// HasDCAdminBinding returns true if any dc-admin binding exists.
func HasDCAdminBinding(ctx context.Context, c client.Client, ns string) (bool, error) {
	var list authv1.FleetRoleBindingList
	if err := c.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return false, err
	}
	for _, rb := range list.Items {
		if rb.Spec.RoleRef.Name == "dc-admin" {
			return true, nil
		}
	}
	return false, nil
}
