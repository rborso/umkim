package httpapi

import (
	"net/http"

	authv1 "github.com/rborso/umkim/controllers/fleet-console-auth/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-console-auth/internal/bootstrap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type bootstrapRequest struct {
	InstallToken string `json:"installToken"`
	Username     string `json:"username"`
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.Cfg.BootstrapInstallToken == "" {
		http.Error(w, "bootstrap not configured", http.StatusNotFound)
		return
	}
	var body bootstrapRequest
	if err := jsonDecode(r.Body, &body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if body.InstallToken != s.Cfg.BootstrapInstallToken {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if body.Username == "" {
		http.Error(w, "username required", http.StatusBadRequest)
		return
	}
	has, err := bootstrap.HasDCAdminBinding(r.Context(), s.Client, s.Cfg.Namespace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if has {
		http.Error(w, "bootstrap already completed", http.StatusConflict)
		return
	}
	rb := authv1.FleetRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-admin-user", Namespace: s.Cfg.Namespace},
		Spec: authv1.FleetRoleBindingSpec{
			Subjects: []authv1.SubjectRef{{Kind: "User", Name: body.Username}},
			RoleRef:  authv1.RoleRef{Name: "dc-admin"},
			Scopes:   []authv1.ScopeRef{{Kind: "*", Name: "*"}},
		},
	}
	if err := s.Client.Create(r.Context(), &rb); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok", "user": body.Username})
}
