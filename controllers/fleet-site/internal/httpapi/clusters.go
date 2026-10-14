package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/registry"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type clusterSummary struct {
	Name                    string                               `json:"name"`
	Site                    string                               `json:"site,omitempty"`
	Available               bool                                 `json:"available"`
	Management              bool                                 `json:"management,omitempty"`
	ReadOnly                bool                                 `json:"read_only,omitempty"`
	Capabilities            map[string]bool                      `json:"capabilities,omitempty"`
	Topology                map[string]string                    `json:"topology,omitempty"`
	IngressEnabled          bool                                 `json:"ingress_enabled,omitempty"`
	IngressAuthEnabled      bool                                 `json:"ingress_auth_enabled,omitempty"`
	DirectIngress           bool                                 `json:"direct_ingress,omitempty"`
	IngressID               string                               `json:"ingress_id,omitempty"`
	Method                  string                               `json:"method,omitempty"`
	TlsPolicy               string                               `json:"tls_policy,omitempty"`
	DNSZone                 string                               `json:"dns_zone,omitempty"`
	EdgeWANIP               string                               `json:"edge_wan_ip,omitempty"`
	UpstreamHost            string                               `json:"upstream_host,omitempty"`
	TraefikHTTPPort         int                                  `json:"traefik_http_port,omitempty"`
	Jumphost                string                               `json:"jumphost,omitempty"`
	ConsoleManaged          bool                                 `json:"console_managed,omitempty"`
	Labels                  map[string]string                    `json:"labels,omitempty"`
	ProvisioningPhase       string                               `json:"provisioning_phase,omitempty"`
	ProvisioningMode        string                               `json:"provisioning_mode,omitempty"`
	ManagedClusterAvailable bool                                 `json:"managed_cluster_available,omitempty"`
	CapabilityDiscovery     *capabilityDiscoverySummary          `json:"capability_discovery,omitempty"`
	Storage                 map[string]interface{}               `json:"storage,omitempty"`
	StorageStatus           *clusterStorageStatusJSON            `json:"storageStatus,omitempty"`
	Providers               map[string]clusterProviderStatusJSON `json:"providers,omitempty"`
	IngressAuthStatus       *clusterIngressAuthStatusJSON        `json:"ingressAuthStatus,omitempty"`
	AcmeEnabled             bool                                 `json:"acme_enabled,omitempty"`
	AcmeStatus              *clusterAcmeStatusJSON               `json:"acmeStatus,omitempty"`
	NodeOperation           *clusterNodeOperationJSON            `json:"nodeOperation,omitempty"`
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
	// Directory is "local" (own LLDAP) or "hub" (shared hub directory).
	Directory string `json:"directory,omitempty"`
}

type clusterStorageStatusJSON struct {
	Phase   string `json:"phase,omitempty"`
	JobName string `json:"jobName,omitempty"`
	Message string `json:"message,omitempty"`
}

type clusterProviderStatusJSON struct {
	Phase        string `json:"phase,omitempty"`
	JobName      string `json:"jobName,omitempty"`
	Message      string `json:"message,omitempty"`
	ProviderName string `json:"providerName,omitempty"`
}

type clusterNodeOperationJSON struct {
	Type      string `json:"type,omitempty"`
	Node      string `json:"node,omitempty"`
	Phase     string `json:"phase,omitempty"`
	JobName   string `json:"jobName,omitempty"`
	Message   string `json:"message,omitempty"`
	Force     bool   `json:"force,omitempty"`
	ID        string `json:"id,omitempty"`
	ActorKind string `json:"actorKind,omitempty"`
	ActorName string `json:"actorName,omitempty"`
}

type clusterPatchBody struct {
	SiteID       string                   `json:"siteId,omitempty"`
	Jumphost     *string                  `json:"jumphost,omitempty"`
	Capabilities map[string]bool          `json:"capabilities,omitempty"`
	Topology     map[string]string        `json:"topology,omitempty"`
	Edge         *clusterEdgePatch        `json:"edge,omitempty"`
	CustomLabels map[string]string        `json:"customLabels,omitempty"`
	IngressAuth  *clusterIngressAuthPatch `json:"ingressAuth,omitempty"`
}

type clusterIngressAuthPatch struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type clusterEdgePatch struct {
	IngressID       string `json:"ingress_id,omitempty"`
	UpstreamHost    string `json:"upstream_host,omitempty"`
	TraefikHTTPPort *int   `json:"traefik_http_port,omitempty"`
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
	if sub == "node-operations/validate" {
		s.handleValidateNodeOperation(w, r, name)
		return
	}
	if sub == "node-operations" || strings.HasPrefix(sub, "node-operations/") {
		rest := strings.TrimPrefix(sub, "node-operations")
		s.handleNodeOperations(w, r, name, rest)
		return
	}
	if sub == "control-plane/replace" {
		s.handleReplaceControlPlane(w, r, name)
		return
	}
	if sub == "nodes" || strings.HasPrefix(sub, "nodes/") {
		s.handleClusterNodes(w, r, name, sub)
		return
	}
	if sub == "vms" || strings.HasPrefix(sub, "vms/") {
		s.handleClusterVMs(w, r, name, sub)
		return
	}
	if sub == "bootstrap-script" && r.Method == http.MethodGet {
		if !s.requireIdentity(w, r) {
			return
		}
		s.serveClusterBootstrapScript(r.Context(), w, name)
		return
	}
	if sub == "bootstrap-bundle" && r.Method == http.MethodGet {
		if !s.requireIdentity(w, r) {
			return
		}
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
	if sub == "bootstrap" && r.Method == http.MethodPost {
		s.handleBootstrapCluster(r.Context(), w, r, name)
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
	if sub == "platform" || strings.HasPrefix(sub, "platform/") {
		rest := strings.TrimPrefix(sub, "platform")
		rest = strings.TrimPrefix(rest, "/")
		s.handleClusterPlatform(w, r, name, rest)
		return
	}
	if sub == "gpu" || strings.HasPrefix(sub, "gpu/") {
		rest := strings.TrimPrefix(sub, "gpu")
		rest = strings.TrimPrefix(rest, "/")
		s.handleClusterGpu(w, r, name, rest)
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
		http.Error(w, "service providers are not used; install Helm with FleetApp", http.StatusGone)
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
	fcjByCluster := s.fleetClusterJoinsByClusterName(ctx)
	for _, mc := range items {
		cs := clusterFromMC(&mc, edge)
		if fcj, ok := fcjByCluster[mc.GetName()]; ok {
			applyFleetClusterJoinSummary(&cs, fcj)
			cs.ManagedClusterAvailable = mcConditionAvailable(&mc)
		}
		out = append(out, cs)
		seen[mc.GetName()] = true
	}
	joins := pendingJoinsFromMap(fcjByCluster, seen)
	for i := range joins {
		out = append(out, clusterSummaryFromJoin(&joins[i]))
		seen[joins[i].Spec.EffectiveClusterName(joins[i].Name)] = true
	}
	// listManagedClusters and pendingJoinsFromMap both iterate maps, so order by
	// name before answering.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, map[string]interface{}{"clusters": out})
}

func pendingJoinsFromMap(fcjByCluster map[string]*fleetv1alpha1.FleetClusterJoin, seen map[string]bool) []fleetv1alpha1.FleetClusterJoin {
	out := make([]fleetv1alpha1.FleetClusterJoin, 0)
	for name, fcj := range fcjByCluster {
		if seen[name] || fcj.Status.Phase == fleetv1alpha1.JoinPhaseReady {
			continue
		}
		out = append(out, *fcj)
	}
	return out
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
	cs := clusterFromMC(mc, edge)
	if fcj, jerr := s.findFleetClusterJoin(ctx, name); jerr == nil {
		applyFleetClusterJoinSummary(&cs, fcj)
		cs.ManagedClusterAvailable = mcConditionAvailable(mc)
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) handlePatchCluster(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	_, ok := s.requireMutatingActor(w, r)
	if !ok {
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
	if patch.Jumphost != nil {
		if err := validateJumphostValue(*patch.Jumphost); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	fcj, fcjErr := s.findFleetClusterJoin(ctx, name)
	notReady := fcjErr == nil && fcj.Status.Phase != "" && fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady
	jumphostOnly := patchJumphostOnly(patch)
	if notReady && !jumphostOnly {
		http.Error(w, "cluster is not provisioned yet", http.StatusConflict)
		return
	}

	if jumphostOnly {
		edgePre, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
		if s.clusterReadOnly(name, edgePre) {
			http.Error(w, "cluster "+name+" is read-only and cannot be modified via the API", http.StatusForbidden)
			return
		}
		if err := s.syncFleetClusterJoinFromPatch(ctx, name, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if fcjErr == nil {
			updated, err := s.findFleetClusterJoin(ctx, name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, clusterSummaryFromJoin(updated))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	edgePre, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if s.clusterReadOnly(name, edgePre) {
		http.Error(w, "cluster "+name+" is read-only and cannot be modified via the API", http.StatusForbidden)
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
			if patch.Edge.IngressID != "" {
				siteID := ent.Site
				if patch.SiteID != "" {
					siteID = patch.SiteID
				}
				if siteID == "" {
					siteID = labels["fleet.site"]
				}
				if err := s.validateClusterIngressBinding(ctx, siteID, name, patch.Edge.IngressID); err != nil {
					writeIngressConflict(w, err)
					return
				}
				site, serr := s.loadSiteForCluster(ctx, siteID)
				if serr != nil {
					http.Error(w, serr.Error(), http.StatusBadRequest)
					return
				}
				resolved, rerr := resolveEdgeFromIngress(site, patch.Edge.IngressID)
				if rerr != nil {
					http.Error(w, rerr.Error(), http.StatusBadRequest)
					return
				}
				ent.IngressID = resolved.IngressID
				ent.Method = resolved.Method
				ent.TlsPolicy = resolved.TlsPolicy
				ent.DirectIngress = resolved.DirectIngress
				ent.DNSZone = resolved.DNSZone
				ent.EdgeWANIP = resolved.EdgeWANIP
				ent.WanAddressMode = resolved.WanAddressMode
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
	cs := clusterFromMC(mc, edgeAfter)
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
	if patch.Edge != nil && patch.Edge.IngressID != "" {
		if fcj.Spec.Edge.IngressId != patch.Edge.IngressID {
			fcj.Spec.Edge.IngressId = patch.Edge.IngressID
			updated = true
		}
	}
	if patch.Jumphost != nil {
		patchJumphost := strings.TrimSpace(*patch.Jumphost)
		if fcj.Spec.Jumphost != patchJumphost {
			fcj.Spec.Jumphost = patchJumphost
			updated = true
		}
	}
	if !updated {
		return nil
	}
	return s.Client.Update(ctx, fcj)
}

func clusterFromMC(mc *unstructured.Unstructured, edge edgeClustersFile) clusterSummary {
	labels := mcLabels(mc)
	cs := clusterSummary{
		Name:                mc.GetName(),
		Site:                labels["fleet.site"],
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
	var edgeEnt registry.ClusterIngressInput
	if e, ok := edge.Clusters[mc.GetName()]; ok {
		cs.Management = e.Management
		cs.ReadOnly = e.ReadOnly
		cs.DirectIngress = e.DirectIngress
		cs.IngressID = e.IngressID
		cs.Method = e.Method
		cs.TlsPolicy = e.TlsPolicy
		cs.DNSZone = e.DNSZone
		cs.EdgeWANIP = e.EdgeWANIP
		cs.UpstreamHost = e.UpstreamHost
		cs.TraefikHTTPPort = e.TraefikHTTPPort
		edgeEnt = registry.ClusterIngressInput{
			IngressID:     e.IngressID,
			Method:        e.Method,
			TlsPolicy:     e.TlsPolicy,
			DirectIngress: e.DirectIngress,
			DNSZone:       e.DNSZone,
			EdgeWANIP:     e.EdgeWANIP,
		}
		if cs.Site == "" {
			cs.Site = e.Site
		}
	}
	if cs.Available {
		cs.ProvisioningPhase = fleetv1alpha1.JoinPhaseReady
	}
	cs.IngressEnabled = registry.ClusterIngressEnabled(edgeEnt, "")
	cs.DirectIngress = registry.DirectIngressFromMethod(cs.Method)
	if cs.TlsPolicy == "" {
		cs.TlsPolicy = registry.EffectiveClusterTLSPolicy(edgeEnt, "")
	}
	return cs
}

func boolLabel(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func allowedCustomLabel(k string) bool {
	return strings.HasPrefix(k, "fleet.custom/")
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
