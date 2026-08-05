package httpapi

import (
	"net/http"
	"strings"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/org"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *Server) handleOrgs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDCAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		var list authv1.FleetOrgList
		if err := s.Client.List(r.Context(), &list, client.InNamespace(s.Cfg.Namespace)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"orgs": list.Items})
	case http.MethodPost:
		var body authv1.FleetOrg
		if err := jsonDecode(r.Body, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		slug := strings.TrimSpace(body.Spec.Slug)
		if slug == "" && body.Name != "" {
			slug = body.Name
		}
		if err := org.ValidateSlug(slug); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		name := body.Name
		if name == "" {
			name = slug
		}
		if name != slug {
			http.Error(w, "metadata.name must equal spec.slug", http.StatusBadRequest)
			return
		}
		o := authv1.FleetOrg{
			ObjectMeta: metav1.ObjectMeta{Name: slug, Namespace: s.Cfg.Namespace},
			Spec: authv1.FleetOrgSpec{
				Slug:        slug,
				DisplayName: body.Spec.DisplayName,
				SharedQuota: body.Spec.SharedQuota,
			},
		}
		if err := s.Client.Create(r.Context(), &o); err != nil {
			if apierrors.IsAlreadyExists(err) {
				http.Error(w, "org already exists", http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, o)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOrgBySlug(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDCAdmin(w, r); !ok {
		return
	}
	slug := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/orgs/")
	slug = strings.Trim(slug, "/")
	if slug == "" {
		http.Error(w, "org slug required", http.StatusBadRequest)
		return
	}
	var o authv1.FleetOrg
	if err := s.Client.Get(r.Context(), types.NamespacedName{Namespace: s.Cfg.Namespace, Name: slug}, &o); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, o)
	case http.MethodPatch:
		var patch authv1.FleetOrg
		if err := jsonDecode(r.Body, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if patch.Spec.Slug != "" && patch.Spec.Slug != o.Spec.Slug {
			http.Error(w, "spec.slug is immutable", http.StatusBadRequest)
			return
		}
		if patch.Spec.DisplayName != "" {
			o.Spec.DisplayName = patch.Spec.DisplayName
		}
		if patch.Spec.SharedQuota != nil {
			o.Spec.SharedQuota = patch.Spec.SharedQuota
		}
		if err := s.Client.Update(r.Context(), &o); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, o)
	case http.MethodDelete:
		if err := s.Client.Delete(r.Context(), &o); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
