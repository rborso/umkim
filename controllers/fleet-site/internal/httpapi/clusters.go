package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/registry"
	"github.com/rborso/umkim/controllers/fleet-site/internal/tenancy"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type clusterSummary struct {
	Name              string            `json:"name"`
	Site              string            `json:"site,omitempty"`
	Pool              string            `json:"pool,omitempty"`
	Available         bool              `json:"available"`
	Management        bool              `json:"management,omitempty"`
	ReadOnly          bool              `json:"read_only,omitempty"`
	Capabilities      map[string]bool   `json:"capabilities,omitempty"`
	Topology          map[string]string `json:"topology,omitempty"`
	IngressEnabled    bool              `json:"ingress_enabled,omitempty"`
	IngressAuthEnabled bool             `json:"ingress_auth_enabled,omitempty"`
	DirectIngress     bool              `json:"direct_ingress,omitempty"`
	ExposureMode      string            `json:"exposure_mode,omitempty"`
	TlsPolicy         string            `json:"tls_policy,omitempty"`
	IngressEndpointID string            `json:"ingress_endpoint_id,omitempty"`
	DomainBindings    []string          `json:"domain_bindings,omitempty"`
	UpstreamHost      string            `json:"upstream_host,omitempty"`
	TraefikHTTPPort   int               `json:"traefik_http_port,omitempty"`
	ConsoleManaged          bool              `json:"console_managed,omitempty"`
	Labels                  map[string]string `json:"labels,omitempty"`
	ProvisioningPhase       string            `json:"provisioning_phase,omitempty"`
	ProvisioningMode        string            `json:"provisioning_mode,omitempty"`
	ManagedClusterAvailable bool                         `json:"managed_cluster_available,omitempty"`
	CapabilityDiscovery     *capabilityDiscoverySummary  `json:"capability_discovery,omitempty"`
	Storage                 map[string]interface{}       `json:"storage,omitempty"`
	StorageStatus           *clusterStorageStatusJSON    `json:"storageStatus,omitempty"`
	IngressAuthStatus       *clusterIngressAuthStatusJSON `json:"ingressAuthStatus,omitempty"`
	AcmeEnabled             bool                          `json:"acme_enabled,omitempty"`
	AcmeStatus              *clusterAcmeStatusJSON        `json:"acmeStatus,omitempty"`
}

type clusterAcmeStatusJSON struct {
	Phase   string `json:"phase,omitempty"`
	JobName string `json:"jobName,omitempty"`
	Message string `json:"message,omitempty"`
}

type clusterIngressAuthStatusJSON struct {
	Phase          string   `json:"phase,omitempty"`
	JobName        string   `json:"jobName,omitempty"`
	Message        string   `json:"message,omitempty"`
	SecretsRef     string   `json:"secretsRef,omitempty"`
	PortalHost     string   `json:"portalHost,omitempty"`
	LLDAPWebUIHost string   `json:"lldapWebUIHost,omitempty"`
	AccessDomains  []string `json:"accessDomains,omitempty"`
}

type clusterStorageStatusJSON struct {
	Phase   string `json:"phase,omitempty"`
	JobName string `json:"jobName,omitempty"`
	Message string `json:"message,omitempty"`
}

type clusterPatchBody struct {
	SiteID       string            `json:"siteId,omitempty"`
	Pool         string            `json:"pool,omitempty"`
	// Org assigns fleet.org (dc-admin only). Empty string clears the label.
	Org          *string           `json:"org,omitempty"`
	OrgId        *string           `json:"orgId,omitempty"`
	Capabilities map[string]bool   `json:"capabilities,omitempty"`
	Topology     map[string]string `json:"topology,omitempty"`
	Edge         *clusterEdgePatch `json:"edge,omitempty"`
	CustomLabels map[string]string `json:"customLabels,omitempty"`
	IngressAuth  *clusterIngressAuthPatch `json:"ingressAuth,omitempty"`
}

type clusterIngressAuthPatch struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type clusterEdgePatch struct {
	DirectIngress     *bool    `json:"direct_ingress,omitempty"`
	ExposureMode      string   `json:"exposure_mode,omitempty"`
	TlsPolicy         string   `json:"tls_policy,omitempty"`
	IngressEndpointID string   `json:"ingress_endpoint_id,omitempty"`
	DomainBindings    []string `json:"domain_bindings,omitempty"`
	UpstreamHost      string   `json:"upstream_host,omitempty"`
	TraefikHTTPPort   *int     `json:"traefik_http_port,omitempty"`
}

func (s *Server) handleClusters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListClusters(r.Context(), w)
	case http.MethodPost:
		s.handleCreateCluster(r.Context(), w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleClusterByName(w http.ResponseWriter, r *http.Request, name, sub string) {
	if sub == "nodes" || strings.HasPrefix(sub, "nodes/") {
		s.handleClusterNodes(w, r, name, sub)
		return
	}
	if sub == "bootstrap-script" && r.Method == http.MethodGet {
		s.serveClusterBootstrapScript(r.Context(), w, name)
		return
	}
	if sub == "bootstrap-bundle" && r.Method == http.MethodGet {
		s.serveClusterBootstrapBundle(r.Context(), w, name)
		return
	}
	if sub == "validate" && r.Method == http.MethodPost {
		s.triggerClusterAction(r.Context(), w, name, "validate")
		return
	}
	if sub == "provision" && r.Method == http.MethodPost {
		s.triggerClusterAction(r.Context(), w, name, "provision")
		return
	}
	if sub == "discover-capabilities" && r.Method == http.MethodPost {
		s.handleDiscoverCapabilities(r.Context(), w, name)
		return
	}
	if sub == "configure-storage" && r.Method == http.MethodPost {
		s.handleConfigureStorage(r.Context(), w, r, name)
		return
	}
	if sub == "ingress-auth" || strings.HasPrefix(sub, "ingress-auth/") {
		rest := strings.TrimPrefix(sub, "ingress-auth")
		rest = strings.TrimPrefix(rest, "/")
		s.handleClusterIngressAuth(w, r, name, rest)
		return
	}
	if sub == "acme" || strings.HasPrefix(sub, "acme/") {
		rest := strings.TrimPrefix(sub, "acme")
		rest = strings.TrimPrefix(rest, "/")
		s.handleClusterAcme(w, r, name, rest)
		return
	}
	if sub == "validate-storage" && r.Method == http.MethodPost {
		s.handleValidateStorage(r.Context(), w, r, name)
		return
	}
	if sub == "adopt" && r.Method == http.MethodPost {
		s.handleAdoptCluster(r.Context(), w, r, name)
		return
	}
	if sub == "providers" || strings.HasPrefix(sub, "providers/") {
		rest := strings.TrimPrefix(sub, "providers")
		rest = strings.TrimPrefix(rest, "/")
		s.handleClusterProviders(w, r, name, rest)
		return
	}
	if sub != "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetCluster(r.Context(), w, name)
	case http.MethodPatch:
		s.handlePatchCluster(r.Context(), w, r, name)
	case http.MethodDelete:
		s.handleDeleteCluster(r.Context(), w, r, name)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleListClusters(ctx context.Context, w http.ResponseWriter) {
	items, err := listManagedClusters(ctx, s.Client)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	seen := map[string]bool{}
	out := make([]clusterSummary, 0, len(items))
	siteDefaults := s.siteDefaultExposureMap(ctx)
	fcjByCluster := s.fleetClusterJoinsByClusterName(ctx)
	for _, mc := range items {
		cs := clusterFromMC(&mc, edge, siteDefaults)
		if fcj, ok := fcjByCluster[mc.GetName()]; ok {
			applyFleetClusterJoinSummary(&cs, fcj)
			cs.ManagedClusterAvailable = mcConditionAvailable(&mc)
		}
		out = append(out, cs)
		seen[mc.GetName()] = true
	}
	joins, _ := s.listPendingClusterJoins(ctx)
	for i := range joins {
		n := joins[i].Spec.EffectiveClusterName(joins[i].Name)
		if seen[n] {
			continue
		}
		out = append(out, clusterSummaryFromJoin(&joins[i]))
		seen[n] = true
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"clusters": out})
}

func (s *Server) handleGetCluster(ctx context.Context, w http.ResponseWriter, name string) {
	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	mc, err := getManagedCluster(ctx, s.Client, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			fcj, jerr := s.getFleetClusterJoin(ctx, name)
			if jerr == nil {
				writeJSON(w, http.StatusOK, clusterSummaryFromJoin(fcj))
				return
			}
			if !apierrors.IsNotFound(jerr) {
				http.Error(w, jerr.Error(), http.StatusInternalServerError)
				return
			}
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	siteDefaults := s.siteDefaultExposureMap(ctx)
	cs := clusterFromMC(mc, edge, siteDefaults)
	if fcj, jerr := s.findFleetClusterJoin(ctx, name); jerr == nil {
		applyFleetClusterJoinSummary(&cs, fcj)
		cs.ManagedClusterAvailable = mcConditionAvailable(mc)
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) handlePatchCluster(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	actor, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	if fcj, err := s.getFleetClusterJoin(ctx, name); err == nil && fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		http.Error(w, "cluster is not provisioned yet", http.StatusConflict)
		return
	}
	edgePre, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if s.clusterReadOnly(name, edgePre) {
		http.Error(w, "cluster "+name+" is read-only and cannot be modified via the API", http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var patch clusterPatchBody
	if err := json.Unmarshal(body, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mc, err := getManagedCluster(ctx, s.Client, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	labels := mcLabels(mc)
	if labels == nil {
		labels = map[string]string{}
	}

	if patch.SiteID != "" {
		labels["fleet.site"] = patch.SiteID
	}
	if len(patch.Capabilities) > 0 {
		http.Error(w, "cluster capabilities are read-only; use POST /clusters/{name}/discover-capabilities", http.StatusBadRequest)
		return
	}
	for k, v := range patch.Topology {
		if k == "ha" {
			labels["fleet.topology/ha"] = v
			continue
		}
		labels["fleet.topology/"+k] = v
	}
	for k, v := range patch.CustomLabels {
		if !allowedCustomLabel(k) {
			http.Error(w, "label not allowed: "+k, http.StatusBadRequest)
			return
		}
		labels[k] = v
	}
	if patch.Pool != "" {
		if !actor.isPlatformAdmin() {
			http.Error(w, "only platform admin may set pool", http.StatusForbidden)
			return
		}
		if patch.Pool != tenancy.PoolShared && patch.Pool != tenancy.PoolDedicated {
			http.Error(w, "pool must be shared or dedicated", http.StatusBadRequest)
			return
		}
		labels[tenancy.LabelPool] = patch.Pool
	}
	orgVal := patch.Org
	if orgVal == nil {
		orgVal = patch.OrgId
	}
	if orgVal != nil {
		if !actor.isPlatformAdmin() {
			http.Error(w, "only platform admin may set org", http.StatusForbidden)
			return
		}
		org := strings.TrimSpace(*orgVal)
		if org == "" {
			delete(labels, tenancy.LabelOrg)
		} else {
			labels[tenancy.LabelOrg] = org
		}
	}

	mc.SetLabels(labels)
	ann := mc.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann["fleet.umkim.com/console-managed"] = "true"
	mc.SetAnnotations(ann)

	if err := s.Client.Update(ctx, mc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.syncFleetClusterJoinFromPatch(ctx, name, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if patch.Edge != nil || patch.SiteID != "" {
		edgeFile, err := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ent := edgeFile.Clusters[name]
		if patch.SiteID != "" {
			ent.Site = patch.SiteID
		}
		if patch.Edge != nil {
			if patch.Edge.ExposureMode != "" {
				ent.ExposureMode = patch.Edge.ExposureMode
			}
			if patch.Edge.TlsPolicy != "" {
				ent.TlsPolicy = patch.Edge.TlsPolicy
			}
			if patch.Edge.IngressEndpointID != "" {
				ent.IngressEndpointID = patch.Edge.IngressEndpointID
			}
			if patch.Edge.DomainBindings != nil {
				ent.DomainBindings = patch.Edge.DomainBindings
			}
			if patch.Edge.DirectIngress != nil {
				ent.DirectIngress = *patch.Edge.DirectIngress
			} else if patch.Edge.ExposureMode != "" {
				ent.DirectIngress = registry.DirectIngressFromExposure(
					registry.EffectiveClusterExposureMode(registry.ClusterExposureInput{
						ExposureMode: patch.Edge.ExposureMode,
					}, ""),
				)
			}
			if patch.Edge.UpstreamHost != "" {
				ent.UpstreamHost = patch.Edge.UpstreamHost
			}
			if patch.Edge.TraefikHTTPPort != nil {
				ent.TraefikHTTPPort = *patch.Edge.TraefikHTTPPort
			}
		}
		if ent.TraefikHTTPPort == 0 {
			ent.TraefikHTTPPort = 30080
		}
		edgeFile.Clusters[name] = ent
		if err := saveEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName, edgeFile); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = registry.SyncSiteAssignedClusters(ctx, s.Client, s.domainRegistryConfig())
	}

	if patch.IngressAuth != nil && patch.IngressAuth.Enabled != nil {
		if *patch.IngressAuth.Enabled {
			http.Error(w, errClusterIngressAuthUseEnable.Error(), http.StatusBadRequest)
			return
		}
		if err := s.patchClusterIngressAuth(ctx, name, false); err != nil {
			if errors.Is(err, errClusterNotReady) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			if apierrors.IsNotFound(err) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	_ = registry.ReconcileEdgeDomains(ctx, s.Client, s.domainRegistryConfig())
	edgeAfter, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	siteDefaults := s.siteDefaultExposureMap(ctx)
	cs := clusterFromMC(mc, edgeAfter, siteDefaults)
	if fcj, jerr := s.findFleetClusterJoin(ctx, name); jerr == nil {
		applyFleetClusterJoinSummary(&cs, fcj)
		cs.ManagedClusterAvailable = mcConditionAvailable(mc)
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) syncFleetClusterJoinFromPatch(ctx context.Context, clusterName string, patch *clusterPatchBody) error {
	if patch == nil {
		return nil
	}
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	updated := false
	if patch.SiteID != "" && fcj.Spec.SiteId != patch.SiteID {
		fcj.Spec.SiteId = patch.SiteID
		updated = true
	}
	if patch.Edge != nil {
		if patch.Edge.ExposureMode != "" && fcj.Spec.Edge.ExposureMode != patch.Edge.ExposureMode {
			fcj.Spec.Edge.ExposureMode = patch.Edge.ExposureMode
			updated = true
		}
		if patch.Edge.TlsPolicy != "" && fcj.Spec.Edge.TlsPolicy != patch.Edge.TlsPolicy {
			fcj.Spec.Edge.TlsPolicy = patch.Edge.TlsPolicy
			updated = true
		}
		if patch.Edge.IngressEndpointID != "" && fcj.Spec.Edge.IngressEndpointId != patch.Edge.IngressEndpointID {
			fcj.Spec.Edge.IngressEndpointId = patch.Edge.IngressEndpointID
			updated = true
		}
		if patch.Edge.DomainBindings != nil {
			fcj.Spec.DomainBindings = append([]string(nil), patch.Edge.DomainBindings...)
			updated = true
		}
	}
	if !updated {
		return nil
	}
	return s.Client.Update(ctx, fcj)
}

func (s *Server) siteDefaultExposureMap(ctx context.Context) map[string]string {
	out := map[string]string{}
	var list fleetv1alpha1.FleetSiteList
	if err := s.Client.List(ctx, &list, client.InNamespace(s.Cfg.Namespace)); err != nil {
		return out
	}
	for i := range list.Items {
		fs := &list.Items[i]
		siteID := fs.Spec.EffectiveSiteID(fs.Name)
		out[siteID] = fs.Spec.EffectiveDefaultExposureMode()
	}
	return out
}

func clusterFromMC(mc *unstructured.Unstructured, edge edgeClustersFile, siteDefaults map[string]string) clusterSummary {
	labels := mcLabels(mc)
	cs := clusterSummary{
		Name:                mc.GetName(),
		Site:                labels["fleet.site"],
		Pool:                labels[tenancy.LabelPool],
		Available:           mcConditionAvailable(mc),
		Capabilities:        registry.CapabilitiesFromLabels(labels),
		Labels:              fleetLabelsFromMC(labels),
		CapabilityDiscovery: discoveryFromMC(mc.GetAnnotations()),
		Topology: map[string]string{
			"region":      labels["fleet.topology/region"],
			"nest_depth":  labels["fleet.topology/nest_depth"],
			"provisioner": labels["fleet.topology/provisioner"],
			"ha":          labels["fleet.topology/ha"],
		},
		ConsoleManaged: labels["fleet.umkim.com/console-managed"] == "true" ||
			(mc.GetAnnotations() != nil && mc.GetAnnotations()["fleet.umkim.com/console-managed"] == "true"),
	}
	var edgeEnt registry.ClusterExposureInput
	if e, ok := edge.Clusters[mc.GetName()]; ok {
		cs.Management = e.Management
		cs.ReadOnly = e.ReadOnly
		cs.DirectIngress = e.DirectIngress
		cs.ExposureMode = e.ExposureMode
		cs.TlsPolicy = e.TlsPolicy
		cs.IngressEndpointID = e.IngressEndpointID
		cs.DomainBindings = e.DomainBindings
		cs.UpstreamHost = e.UpstreamHost
		cs.TraefikHTTPPort = e.TraefikHTTPPort
		edgeEnt = registry.ClusterExposureInput{
			ExposureMode:      e.ExposureMode,
			TlsPolicy:         e.TlsPolicy,
			DirectIngress:     e.DirectIngress,
			IngressEndpointID: e.IngressEndpointID,
			DomainBindings:    e.DomainBindings,
		}
		if cs.Site == "" {
			cs.Site = e.Site
		}
	}
	if cs.Available {
		cs.ProvisioningPhase = fleetv1alpha1.JoinPhaseReady
	}
	siteDefault := siteDefaults[cs.Site]
	effectiveMode := registry.EffectiveClusterExposureMode(edgeEnt, siteDefault)
	if cs.ExposureMode == "" {
		cs.ExposureMode = effectiveMode
	}
	cs.IngressEnabled = registry.ClusterIngressEnabled(edgeEnt, siteDefault)
	cs.DirectIngress = registry.DirectIngressFromExposure(effectiveMode)
	return cs
}

func boolLabel(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func allowedCustomLabel(k string) bool {
	return strings.HasPrefix(k, "fleet.custom/") || strings.HasPrefix(k, "fleet.") && k != tenancy.LabelPool && k != tenancy.LabelOrg
}

func fleetLabelsFromMC(labels map[string]string) map[string]string {
	if labels == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		if strings.HasPrefix(k, "fleet.") {
			out[k] = v
		}
	}
	return out
}
