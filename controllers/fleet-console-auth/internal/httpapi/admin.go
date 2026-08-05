package httpapi

import (
	"net/http"
	"strings"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/authorizer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDCAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		var list authv1.FleetUserList
		if err := s.Client.List(r.Context(), &list, client.InNamespace(s.Cfg.Namespace)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"users": list.Items})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUserByName(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDCAdmin(w, r); !ok {
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/users/")
	name = strings.Trim(name, "/")
	var u authv1.FleetUser
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &u); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, u)
	case http.MethodPatch:
		var patch authv1.FleetUser
		if err := jsonDecode(r.Body, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if patch.Spec.Disabled {
			u.Spec.Disabled = true
		}
		if patch.Spec.DisplayName != "" {
			u.Spec.DisplayName = patch.Spec.DisplayName
		}
		if err := s.Client.Update(r.Context(), &u); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, u)
	case http.MethodDelete:
		_ = s.Client.Delete(r.Context(), &u)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDCAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		var list authv1.FleetGroupList
		_ = s.Client.List(r.Context(), &list, client.InNamespace(s.Cfg.Namespace))
		writeJSON(w, http.StatusOK, map[string]interface{}{"groups": list.Items})
	case http.MethodPost:
		var g authv1.FleetGroup
		if err := jsonDecode(r.Body, &g); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if g.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		g.Namespace = s.Cfg.Namespace
		if err := s.Client.Create(r.Context(), &g); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusCreated, g)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGroupByName(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDCAdmin(w, r); !ok {
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/groups/")
	name = strings.Trim(name, "/")
	var g authv1.FleetGroup
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &g); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, g)
	case http.MethodDelete:
		_ = s.Client.Delete(r.Context(), &g)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRoleBindings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDCAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		var list authv1.FleetRoleBindingList
		_ = s.Client.List(r.Context(), &list, client.InNamespace(s.Cfg.Namespace))
		writeJSON(w, http.StatusOK, map[string]interface{}{"roleBindings": list.Items})
	case http.MethodPost:
		var rb authv1.FleetRoleBinding
		if err := jsonDecode(r.Body, &rb); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if rb.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		rb.Namespace = s.Cfg.Namespace
		if err := s.Client.Create(r.Context(), &rb); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusCreated, rb)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRoleBindingByName(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDCAdmin(w, r); !ok {
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/role-bindings/")
	name = strings.Trim(name, "/")
	var rb authv1.FleetRoleBinding
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &rb); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, rb)
	case http.MethodPatch:
		var patch authv1.FleetRoleBinding
		if err := jsonDecode(r.Body, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if patch.Spec.Subjects != nil {
			rb.Spec.Subjects = patch.Spec.Subjects
		}
		if patch.Spec.Scopes != nil {
			rb.Spec.Scopes = patch.Spec.Scopes
		}
		if patch.Spec.RoleRef.Name != "" {
			rb.Spec.RoleRef = patch.Spec.RoleRef
		}
		_ = s.Client.Update(r.Context(), &rb)
		writeJSON(w, http.StatusOK, rb)
	case http.MethodDelete:
		_ = s.Client.Delete(r.Context(), &rb)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleServiceAccounts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDCAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		var list authv1.FleetServiceAccountList
		_ = s.Client.List(r.Context(), &list, client.InNamespace(s.Cfg.Namespace))
		writeJSON(w, http.StatusOK, map[string]interface{}{"serviceAccounts": list.Items})
	case http.MethodPost:
		var sa authv1.FleetServiceAccount
		if err := jsonDecode(r.Body, &sa); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if sa.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		sa.Namespace = s.Cfg.Namespace
		if err := s.Client.Create(r.Context(), &sa); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		for _, rb := range sa.Spec.RoleBindings {
			binding := authv1.FleetRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sa.Name + "-" + rb.RoleRef.Name,
					Namespace: s.Cfg.Namespace,
				},
				Spec: authv1.FleetRoleBindingSpec{
					Subjects: []authv1.SubjectRef{{Kind: "ServiceAccount", Name: sa.Name}},
					RoleRef:  rb.RoleRef,
					Scopes:   rb.Scopes,
				},
			}
			_ = s.Client.Create(r.Context(), &binding)
		}
		token, err := s.Tokens.Issue(r.Context(), sa.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"serviceAccount": sa,
			"token":          token,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleServiceAccountByName(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDCAdmin(w, r); !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/service-accounts/")
	path = strings.Trim(path, "/")
	parts := strings.SplitN(path, "/", 2)
	name := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	if action == "rotate-token" && r.Method == http.MethodPost {
		token, err := s.Tokens.Issue(r.Context(), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"token": token})
		return
	}
	var sa authv1.FleetServiceAccount
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &sa); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, sa)
	case http.MethodDelete:
		_ = s.Client.Delete(r.Context(), &sa)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCatalogAccess(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.resolveActor(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	isAdmin, _ := s.RBAC.IsDCAdmin(r.Context(), actor)
	switch r.Method {
	case http.MethodGet:
		if !isAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var list authv1.FleetCatalogAccessList
		_ = s.Client.List(r.Context(), &list, client.InNamespace(s.Cfg.Namespace))
		writeJSON(w, http.StatusOK, map[string]interface{}{"catalogAccess": list.Items})
	case http.MethodPost:
		if !isAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var ca authv1.FleetCatalogAccess
		if err := jsonDecode(r.Body, &ca); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if ca.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		ca.Namespace = s.Cfg.Namespace
		if err := s.Client.Create(r.Context(), &ca); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusCreated, ca)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCatalogAccessByName(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.resolveActor(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	isAdmin, _ := s.RBAC.IsDCAdmin(r.Context(), actor)
	name := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/catalog-access/")
	name = strings.Trim(name, "/")
	var ca authv1.FleetCatalogAccess
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &ca); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, ca)
	case http.MethodPatch:
		if !isAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var patch authv1.FleetCatalogAccess
		if err := jsonDecode(r.Body, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if patch.Spec.Visibility != "" {
			ca.Spec.Visibility = patch.Spec.Visibility
		}
		if patch.Spec.Grants != nil {
			ca.Spec.Grants = patch.Spec.Grants
		}
		_ = s.Client.Update(r.Context(), &ca)
		writeJSON(w, http.StatusOK, ca)
	case http.MethodDelete:
		if !isAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_ = s.Client.Delete(r.Context(), &ca)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAuditEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDCAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var list authv1.FleetAuditEventList
	_ = s.Client.List(r.Context(), &list, client.InNamespace(s.Cfg.Namespace))
	writeJSON(w, http.StatusOK, map[string]interface{}{"auditEvents": list.Items})
	actor, _ := s.resolveActor(r)
	_ = s.Audit.Record(r.Context(), authorizer.SubjectRefFromActor(actor), "audit.list", "allow", nil, nil, httpInfo(r), "")
}
