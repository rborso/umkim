package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/registry"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Server struct {
	Client     client.Client
	// APIReader is a direct (uncached) reader for freshly written objects.
	APIReader  client.Reader
	Cfg        config.Config
	RESTConfig *rest.Config
}

// reader returns the direct API reader when available so freshly created
// objects (invites, password Jobs) are observed without cache lag.
func (s *Server) reader() client.Reader {
	if s.APIReader != nil {
		return s.APIReader
	}
	return s.Client
}

type enrollRequest struct {
	Hostname      string `json:"hostname"`
	RockyVersion  string `json:"rockyVersion"`
	WgPublicKey   string `json:"wgPublicKey"`
	ScriptVersion string `json:"scriptVersion"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1alpha1/site-enrollments/", s.handleEnrollment)
	mux.HandleFunc("/api/v1alpha1/cluster-enrollments/", s.handleClusterEnrollment)
	mux.HandleFunc("/api/v1alpha1/sites", s.handleSites)
	mux.HandleFunc("/api/v1alpha1/sites/", s.handleSiteByName)
	mux.HandleFunc("/api/v1alpha1/infrastructure/", s.handleInfrastructure)
	mux.HandleFunc("/api/v1alpha1/clusters", s.handleClusters)
	mux.HandleFunc("/api/v1alpha1/clusters/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/clusters/")
		path = strings.Trim(path, "/")
		if path == "" {
			http.Error(w, "cluster name required", http.StatusBadRequest)
			return
		}
		parts := strings.SplitN(path, "/", 2)
		name := parts[0]
		sub := ""
		if len(parts) == 2 {
			sub = parts[1]
		}
		s.handleClusterByName(w, r, name, sub)
	})
	mux.HandleFunc("/api/v1alpha1/edge/domains", s.handleEdgeDomains)
	mux.HandleFunc("/api/v1alpha1/edge/hostnames/check", s.handleEdgeDomains)
	mux.HandleFunc("/api/v1alpha1/edge/dns/validate", s.handleEdgeDomains)
	mux.HandleFunc("/api/v1alpha1/ingress-auth", s.handleIngressAuth)
	mux.HandleFunc("/api/v1alpha1/ingress-auth/", func(w http.ResponseWriter, r *http.Request) {
		sub := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/ingress-auth/")
		sub = strings.Trim(sub, "/")
		s.handleIngressAuthSub(w, r, sub)
	})
	mux.HandleFunc("/api/v1alpha1/acme", s.handleAcme)
	mux.HandleFunc("/api/v1alpha1/acme/", func(w http.ResponseWriter, r *http.Request) {
		sub := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/acme/")
		sub = strings.Trim(sub, "/")
		s.handleAcmeSub(w, r, sub)
	})
	mux.HandleFunc("/api/v1alpha1/llm", s.handleLLM)
	mux.HandleFunc("/api/v1alpha1/llm/", func(w http.ResponseWriter, r *http.Request) {
		sub := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/llm/")
		sub = strings.Trim(sub, "/")
		s.handleLLMSub(w, r, sub)
	})
	mux.HandleFunc("/api/v1alpha1/apps", s.handleApps)
	mux.HandleFunc("/api/v1alpha1/apps/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/apps/")
		name = strings.Trim(name, "/")
		if name == "" {
			s.handleApps(w, r)
			return
		}
		s.handleAppByName(w, r, name)
	})
	mux.HandleFunc("/api/v1alpha1/charts", s.handleCharts)
	mux.HandleFunc("/api/v1alpha1/llama-profiles", s.handleLlamaProfiles)
	mux.HandleFunc("/api/v1alpha1/llama-profiles/", func(w http.ResponseWriter, r *http.Request) {
		sub := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/llama-profiles/")
		s.handleLlamaProfilesSub(w, r, sub)
	})
	mux.HandleFunc("/api/v1alpha1/charts/", func(w http.ResponseWriter, r *http.Request) {
		sub := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/charts/")
		sub = strings.Trim(sub, "/")
		if sub == "" {
			s.handleCharts(w, r)
			return
		}
		if strings.HasPrefix(sub, "llama-cpp-helm/profiles") {
			s.handleLlamaCppProfiles(w, r, sub)
			return
		}
		s.handleChartByName(w, r, sub)
	})
	mux.HandleFunc("/api/v1alpha1/invites", s.handleInvites)
	mux.HandleFunc("/api/v1alpha1/invites/", func(w http.ResponseWriter, r *http.Request) {
		sub := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/invites/"), "/")
		if sub == "" {
			s.handleInvites(w, r)
			return
		}
		s.handleInvitesSub(w, r, sub)
	})
	mux.HandleFunc("/api/v1alpha1/tenants", s.handleTenants)
	mux.HandleFunc("/api/v1alpha1/tenants/", func(w http.ResponseWriter, r *http.Request) {
		sub := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/tenants/"), "/")
		if sub == "" {
			s.handleTenants(w, r)
			return
		}
		s.handleTenantsSub(w, r, sub)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

func (s *Server) handleEnrollment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	siteName := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/site-enrollments/")
	siteName = strings.Trim(siteName, "/")
	if siteName == "" {
		http.Error(w, "site name required", http.StatusBadRequest)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}

	var body enrollRequest
	_ = json.NewDecoder(r.Body).Decode(&body)

	ctx := r.Context()
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "site not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !tokenMatches(ctx, s.Client, s.Cfg.Namespace, siteName, token) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	secretPub := strings.TrimSpace(secretWgPublicKey(ctx, s.Client, s.Cfg.Namespace, siteName))
	if body.WgPublicKey != "" {
		switch {
		case secretPub != "" && body.WgPublicKey == secretPub:
			fs.Status.WireguardPublicKey = secretPub
		case fs.Status.WireguardPublicKey != "" && body.WgPublicKey != fs.Status.WireguardPublicKey:
			http.Error(w, "public key mismatch", http.StatusBadRequest)
			return
		case fs.Status.WireguardPublicKey == "":
			fs.Status.WireguardPublicKey = body.WgPublicKey
		}
	}

	now := metav1.Now()
	if fs.Status.Phase == "Enrolled" || fs.Status.Phase == "Validated" || fs.Status.Phase == "Provisioning" || fs.Status.Phase == "Ready" {
		writeJSON(w, http.StatusOK, map[string]string{"phase": fs.Status.Phase, "message": "already enrolled"})
		return
	}
	fs.Status.Phase = "Enrolled"
	fs.Status.Message = "Bootstrap script completed enrollment"
	fs.Status.EnrolledAt = &now
	fs.Status.EnrolledHostname = body.Hostname
	if err := s.Client.Status().Update(ctx, &fs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"phase": "Enrolled"})
}

func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sites, err := s.listSitesEnriched(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"sites": sites})
	case http.MethodPost:
		_, ok := s.requireMutatingActor(w, r)
		if !ok {
			return
		}
		var fs fleetv1alpha1.FleetSite
		if err := json.NewDecoder(r.Body).Decode(&fs); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if fs.Name == "" {
			fs.Name = fs.Spec.SiteName
		}
		if fs.Namespace == "" {
			fs.Namespace = s.Cfg.Namespace
		}
		fs.Spec.NormalizeIngress()
		if err := validateSiteIngressRows(fs.Spec.Edge, fs.Spec.Edge.Ingress); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if fs.Spec.ProvisioningMode == "" {
			fs.Spec.ProvisioningMode = fleetv1alpha1.ProvisioningModeRegistry
		}
		if fs.Spec.SiteName == "" {
			if fs.Spec.SiteId != "" {
				fs.Spec.SiteName = fs.Spec.SiteId
			} else {
				fs.Spec.SiteName = fs.Name
			}
		}
		if fs.Spec.SiteId == "" {
			fs.Spec.SiteId = fs.Spec.SiteName
		}
		normalizeSiteIDOnCreate(&fs)
		if fs.Spec.IsRegistry() {
			fs.Status.Phase = "Ready"
			fs.Status.Message = "Registry site"
		} else {
			fs.Status.Phase = "Draft"
		}
		if err := s.Client.Create(r.Context(), &fs); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		_ = registry.ReconcileEdgeDomains(r.Context(), s.Client, s.domainRegistryConfig())
		writeJSON(w, http.StatusCreated, s.enrichSite(r.Context(), &fs))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSiteByName(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/sites/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	name := parts[0]
	rest := ""
	if len(parts) > 1 {
		rest = strings.Join(parts[1:], "/")
	}

	ctx := r.Context()
	switch {
	case rest == "bootstrap-script" && r.Method == http.MethodGet:
		if s.siteIsManagement(ctx, name) {
			http.Error(w, "management site does not support bootstrap actions", http.StatusForbidden)
			return
		}
		if !s.requireIdentity(w, r) {
			return
		}
		s.serveBootstrapScript(ctx, w, name)
	case rest == "validate" && r.Method == http.MethodPost:
		if s.siteIsManagement(ctx, name) {
			http.Error(w, "management site does not support greenfield actions", http.StatusForbidden)
			return
		}
		s.triggerAction(ctx, w, name, "validate")
	case rest == "provision" && r.Method == http.MethodPost:
		if s.siteIsManagement(ctx, name) {
			http.Error(w, "management site does not support greenfield actions", http.StatusForbidden)
			return
		}
		s.triggerAction(ctx, w, name, "provision")
	case rest == "provision" || strings.HasPrefix(rest, "provision/"):
		s.handleSiteProvisionRoutes(ctx, w, r, name, rest)
	case rest == "hosts" || strings.HasPrefix(rest, "hosts/"):
		s.handleSiteHostRoutes(ctx, w, r, name, rest)
	case rest == "ingress/verify" && r.Method == http.MethodPost:
		s.verifySiteIngress(ctx, w, r, name)
	case rest == "issue-script" && r.Method == http.MethodPost:
		if s.siteIsManagement(ctx, name) {
			http.Error(w, "management site does not support greenfield actions", http.StatusForbidden)
			return
		}
		s.triggerAction(ctx, w, name, "issue-script")
	case rest == "":
		s.handleOneSite(ctx, w, r, name)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleOneSite(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &fs); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.enrichSite(ctx, &fs))
	case http.MethodPatch:
		s.handlePatchSite(ctx, w, r, name)
	case http.MethodDelete:
		_, ok := s.requireMutatingActor(w, r)
		if !ok {
			return
		}
		if s.siteIsManagement(ctx, name) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "management site cannot be deleted"})
			return
		}
		if !fs.Spec.IsRegistry() {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only registry sites may be deleted via API"})
			return
		}
		mcMap, _ := registry.ClustersBySiteID(ctx, s.Client)
		if len(mcMap[fs.Spec.EffectiveSiteID(fs.Name)]) > 0 {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "site has attached clusters"})
			return
		}
		if err := s.Client.Delete(ctx, &fs); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = registry.ReconcileEdgeDomains(ctx, s.Client, s.domainRegistryConfig())
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) serveBootstrapScript(ctx context.Context, w http.ResponseWriter, name string) {
	var secret corev1.Secret
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: bootstrapSecretName(name)}, &secret); err != nil {
		http.Error(w, "script not issued", http.StatusNotFound)
		return
	}
	script := string(secret.Data["bootstrap.sh"])
	w.Header().Set("Content-Type", "text/x-shellscript")
	w.Header().Set("Content-Disposition", "attachment; filename=bootstrap.sh")
	_, _ = io.WriteString(w, script)
}

func (s *Server) triggerAction(ctx context.Context, w http.ResponseWriter, name, action string) {
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &fs); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if fs.Annotations == nil {
		fs.Annotations = map[string]string{}
	}
	fs.Annotations["fleet.umkim.com/"+action] = time.Now().UTC().Format(time.RFC3339)
	if err := s.Client.Update(ctx, &fs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"action": action})
}

func bootstrapSecretName(siteName string) string {
	return "fleet-site-" + siteName + "-bootstrap"
}

func secretWgPublicKey(ctx context.Context, c client.Client, ns, siteName string) string {
	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: bootstrapSecretName(siteName)}, &secret); err != nil {
		return ""
	}
	return string(secret.Data["wg-public-key"])
}

func tokenMatches(ctx context.Context, c client.Client, ns, siteName, token string) bool {
	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: bootstrapSecretName(siteName)}, &secret); err != nil {
		return false
	}
	stored := strings.TrimSpace(string(secret.Data["enrollment-token"]))
	if stored == token {
		return true
	}
	sum := sha256.Sum256([]byte(token))
	return stored == hex.EncodeToString(sum[:])
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// normalizeSiteIDOnCreate repairs console first-keystroke siteId latch (siteId=h, siteName=homelab).
func normalizeSiteIDOnCreate(fs *fleetv1alpha1.FleetSite) {
	if len(fs.Spec.SiteId) == 1 && len(fs.Spec.SiteName) > 1 &&
		strings.HasPrefix(fs.Spec.SiteName, fs.Spec.SiteId) {
		fs.Spec.SiteId = fs.Spec.SiteName
	}
}
