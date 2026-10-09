package ingressauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/hubwan"
	"github.com/rborso/umkim/controllers/fleet-site/internal/hubacme"
	"github.com/rborso/umkim/controllers/fleet-site/internal/teardown"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	ReconcileAnnotation = "fleet.umkim.com/ingress-auth-reconcile"
	InventoryCMName     = "fleet-ingress-auth-inventory"
)

type DesiredState struct {
	HubEnabled           bool
	ControllerAuth       bool
	EnabledClusters      []string
	TeardownClusters     []string
	Phase6Clusters       []string
	PortalHost           string
	CookieDomain         string
	AccessDomains        []string
	GlobalEnabled        bool
	ClusterConfigs       map[string]ClusterAuthConfig
}

type ClusterAuthConfig struct {
	Name              string
	PortalHost        string
	CookieDomain      string
	AccessDomains     []string
	SecretsRef        string
	ExposeLLDAPWebUI  bool
	LLDAPWebUIHost    string
	// Directory is "" / "local" (own LLDAP) or "hub" (shared hub directory).
	Directory         string
	// SpokeTLS is false for reverse_proxy clusters (external TLS), so the Authelia
	// portal is published on the plain HTTP entrypoint for the operator proxy.
	SpokeTLS          bool
}

func EnsureSecret(ctx context.Context, c client.Client, ns, name string) error {
	var existing corev1.Secret
	err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name": "fleet-ingress-auth",
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"bootstrapPassword":     randToken(24),
			"lldapJwtSecret":        randToken(32),
			"sessionSecret":         randToken(32),
			"storageEncryptionKey":  randToken(32),
			"resetJwtSecret":        randToken(32),
		},
	}
	return c.Create(ctx, secret)
}

func randToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return base64.RawURLEncoding.EncodeToString([]byte("fleet-ingress-auth-fallback-token"))
	}
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}

func EnsureCR(ctx context.Context, c client.Client, ns string, cfg config.Config) (*fleetv1alpha1.FleetIngressAuth, error) {
	var fia fleetv1alpha1.FleetIngressAuth
	err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: fleetv1alpha1.FleetIngressAuthName}, &fia)
	if err == nil {
		return &fia, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}
	fia = fleetv1alpha1.FleetIngressAuth{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fleetv1alpha1.FleetIngressAuthName,
			Namespace: ns,
		},
		Spec: fleetv1alpha1.FleetIngressAuthSpec{
			PortalHost:   defaultPortalHost(cfg),
			CookieDomain: cfg.HubEdgeDomain,
			AccessDomains: defaultAccessDomains(cfg),
			SecretsRef:   fleetv1alpha1.DefaultIngressAuthSecretsName,
		},
		Status: fleetv1alpha1.FleetIngressAuthStatus{
			Phase: fleetv1alpha1.IngressAuthPhaseDisabled,
		},
	}
	if err := c.Create(ctx, &fia); err != nil {
		return nil, err
	}
	return &fia, nil
}

func defaultPortalHost(cfg config.Config) string {
	domain := cfg.HubEdgeDomain
	if domain == "" {
		domain = "mgmt.umkim.com"
	}
	return "auth." + domain
}

func defaultAccessDomains(cfg config.Config) []string {
	domain := cfg.HubEdgeDomain
	if domain == "" {
		domain = "mgmt.umkim.com"
	}
	return []string{"*." + domain, domain}
}

func ComputeDesired(ctx context.Context, c client.Client, cfg config.Config, fia *fleetv1alpha1.FleetIngressAuth) (DesiredState, error) {
	out := DesiredState{
		HubEnabled:     fia.Spec.HubEnabled,
		PortalHost:     fia.Spec.PortalHost,
		CookieDomain:   fia.Spec.CookieDomain,
		AccessDomains:  append([]string(nil), fia.Spec.AccessDomains...),
		ClusterConfigs: map[string]ClusterAuthConfig{},
	}
	if out.PortalHost == "" {
		out.PortalHost = defaultPortalHost(cfg)
	}
	if out.CookieDomain == "" {
		out.CookieDomain = cfg.HubEdgeDomain
	}
	if len(out.AccessDomains) == 0 {
		out.AccessDomains = defaultAccessDomains(cfg)
	}

	var joins fleetv1alpha1.FleetClusterJoinList
	if err := c.List(ctx, &joins, client.InNamespace(cfg.Namespace)); err != nil {
		return out, err
	}
	enabledSet := map[string]struct{}{}
	accessUnion := map[string]struct{}{}
	for _, d := range out.AccessDomains {
		accessUnion[d] = struct{}{}
	}
	for _, fcj := range joins.Items {
		name := fcj.Spec.EffectiveClusterName(fcj.Name)
		if !fcj.Spec.IngressAuth.Enabled || fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
			continue
		}
		secretRef := fcj.Status.IngressAuth.SecretsRef
		if secretRef == "" {
			secretRef = ClusterSecretName(name)
		}
		if !ClusterSecretExists(ctx, c, cfg.Namespace, name) {
			continue
		}
		dns := DNSFromFCJStatus(&fcj)
		if resolved, err := ResolveClusterDNS(ctx, c, cfg, name, &fcj); err == nil {
			dns = resolved
		}
		if dns.PortalHost == "" {
			continue
		}
		enabledSet[name] = struct{}{}
		out.EnabledClusters = append(out.EnabledClusters, name)
		cfgEntry := ClusterAuthConfig{
			Name:             name,
			PortalHost:       dns.PortalHost,
			CookieDomain:     dns.PrimaryZone,
			AccessDomains:    dns.AccessDomains,
			SecretsRef:       secretRef,
			ExposeLLDAPWebUI: fcj.Spec.IngressAuth.ExposeLLDAPWebUI,
			Directory:        fcj.Spec.IngressAuth.Directory,
			SpokeTLS:         dns.SpokeTLS,
		}
		if fcj.Spec.IngressAuth.ExposeLLDAPWebUI {
			cfgEntry.LLDAPWebUIHost = dns.LLDAPWebUIHost
			if cfgEntry.LLDAPWebUIHost == "" {
				cfgEntry.LLDAPWebUIHost = LLDAPWebUIHost(dns.PrimaryZone)
			}
		}
		out.ClusterConfigs[name] = cfgEntry
		for _, d := range dns.AccessDomains {
			accessUnion[d] = struct{}{}
		}
	}
	out.EnabledClusters = sortedKeys(enabledSet)
	out.AccessDomains = sortedKeys(accessUnion)

	edge, err := loadEdgeClusters(ctx, c, cfg.EdgeClustersCMNamespace, cfg.EdgeClustersCMName)
	if err != nil {
		return out, err
	}
	for name := range edge.Clusters {
		if name == cfg.ManagementClusterName {
			continue
		}
		ent := edge.Clusters[name]
		if ent.Management || ent.ReadOnly {
			continue
		}
		if !teardown.ManagedClusterExists(ctx, c, name) {
			continue
		}
		out.Phase6Clusters = append(out.Phase6Clusters, name)
	}
	out.Phase6Clusters = sortedKeys(sliceToSet(out.Phase6Clusters))

	prevEnabled := map[string]struct{}{}
	for _, cs := range fia.Status.Clusters {
		if cs.Phase == fleetv1alpha1.IngressAuthClusterPhaseReady || cs.Phase == fleetv1alpha1.IngressAuthClusterPhaseApplying {
			prevEnabled[cs.Name] = struct{}{}
		}
	}
	for name := range prevEnabled {
		if _, ok := enabledSet[name]; !ok {
			out.TeardownClusters = append(out.TeardownClusters, name)
		}
	}
	out.TeardownClusters = sortedKeys(sliceToSet(out.TeardownClusters))

	out.ControllerAuth = out.HubEnabled || len(out.EnabledClusters) > 0
	out.GlobalEnabled = out.ControllerAuth
	return out, nil
}

func BuildInventoryYAML(cfg config.Config, desired DesiredState, fia *fleetv1alpha1.FleetIngressAuth, edgeClusters map[string]AnsibleEdgeCluster) string {
	gatewayURL := "http://127.0.0.1:80"
	if desired.ControllerAuth {
		gatewayURL = "https://127.0.0.1"
	}
	forwardAuth := fmt.Sprintf("http://authelia.%s.svc.cluster.local:9091/api/authz/forward-auth", "fleet-ingress-auth")
	lines := []string{
		"fleet_ingress_auth_enabled: " + boolYAML(desired.GlobalEnabled),
		"fleet_ingress_auth_hub_enabled: " + boolYAML(desired.HubEnabled),
		"fleet_ingress_auth_reconcile_phase6: true",
		"fleet_ingress_auth_reconcile_fleet_edge: true",
		"fleet_ingress_auth_reconcile_controller: true",
		"fleet_ingress_auth_warm_harbor: false",
		"fleet_edge_enable: true",
		"fleet_ingress_auth_portal_host: \"" + yamlQuote(desired.PortalHost) + "\"",
		"fleet_ingress_auth_cookie_domain: \"" + yamlQuote(desired.CookieDomain) + "\"",
		"fleet_ingress_auth_forward_auth_url: \"" + yamlQuote(forwardAuth) + "\"",
		"fleet_edge_hub_gateway_url: \"" + yamlQuote(gatewayURL) + "\"",
		"fleet_edge_domain: \"" + yamlQuote(cfg.HubEdgeDomain) + "\"",
		"fleet_edge_acme_enabled: " + boolYAML(cfg.FleetEdgeACMEEnabled),
		"fleet_edge_acme_email: \"" + yamlQuote(cfg.FleetEdgeACMEEmail) + "\"",
		"harbor_host: \"" + yamlQuote(cfg.HubWireguardIP) + "\"",
		"harbor_nodeport_https: \"30443\"",
		"harbor_proxy_project_ghcr: ghcr-proxy",
		"ocm_kubeconfig: /etc/kube/config",
		"k3s_command_path: /usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"fleet_ingress_auth_chart_src: /ansible/charts/fleet-ingress-auth",
		"fleet_edge_controller_playbook_dir: /ansible",
		"fleet_console_namespace: fleet-console",
		"fleet_auth_lldap_ldap_port: \"3890\"",
		"fleet_ingress_auth_lldap_base_dn: \""+yamlQuote(cfg.HubIngressAuthLDAPBaseDN)+"\"",
		"fleet_ingress_auth_lldap_admin_email: \"admin@"+yamlQuote(cfg.HubEdgeDomain)+"\"",
		"fleet_ingress_auth_bootstrap_password: \"{{ lookup('file', '/secrets/ingress-auth/bootstrapPassword') | trim }}\"",
		"fleet_ingress_auth_lldap_jwt_secret: \"{{ lookup('file', '/secrets/ingress-auth/lldapJwtSecret') | trim }}\"",
		"fleet_ingress_auth_session_secret: \"{{ lookup('file', '/secrets/ingress-auth/sessionSecret') | trim }}\"",
		"fleet_ingress_auth_storage_encryption_key: \"{{ lookup('file', '/secrets/ingress-auth/storageEncryptionKey') | trim }}\"",
		"fleet_ingress_auth_reset_jwt_secret: \"{{ lookup('file', '/secrets/ingress-auth/resetJwtSecret') | trim }}\"",
	}
	if len(desired.AccessDomains) > 0 {
		lines = append(lines, "fleet_ingress_auth_access_domains:")
		for _, d := range desired.AccessDomains {
			lines = append(lines, "  - \""+yamlQuote(d)+"\"")
		}
	}
	if len(desired.ClusterConfigs) > 0 {
		lines = append(lines, "fleet_ingress_auth_cluster_configs:")
		for _, name := range sortedKeys(sliceToSet(desired.EnabledClusters)) {
			cc := desired.ClusterConfigs[name]
			lines = append(lines, "  "+name+":")
			lines = append(lines, "    portal_host: \""+yamlQuote(cc.PortalHost)+"\"")
			lines = append(lines, "    cookie_domain: \""+yamlQuote(cc.CookieDomain)+"\"")
			lines = append(lines, "    secrets_dir: \"/secrets/ingress-auth/clusters/"+name+"\"")
			if len(cc.AccessDomains) > 0 {
				lines = append(lines, "    access_domains:")
				for _, d := range cc.AccessDomains {
					lines = append(lines, "      - \""+yamlQuote(d)+"\"")
				}
			}
			lines = append(lines, "    expose_lldap_webui: "+boolYAML(cc.ExposeLLDAPWebUI))
			lines = append(lines, "    spoke_tls: "+boolYAML(cc.SpokeTLS))
			lines = append(lines, "    directory: \""+yamlQuote(cc.Directory)+"\"")
			if cc.ExposeLLDAPWebUI && cc.LLDAPWebUIHost != "" {
				lines = append(lines, "    lldap_webui_host: \""+yamlQuote(cc.LLDAPWebUIHost)+"\"")
			}
		}
	}
	lines = append(lines, yamlStringList("fleet_ingress_auth_spoke_clusters", desired.EnabledClusters)...)
	lines = append(lines, yamlStringList("fleet_ingress_auth_teardown_spoke_clusters", desired.TeardownClusters)...)
	lines = append(lines, yamlStringList("ocm_manifestwork_phase6_clusters", desired.Phase6Clusters)...)
	if len(edgeClusters) > 0 {
		lines = append(lines, "fleet_edge_clusters:")
		for _, name := range sortedKeys(sliceToSet(keysOfEdgeClusters(edgeClusters))) {
			ent := edgeClusters[name]
			lines = append(lines, "  "+name+":")
			lines = append(lines, "    direct_ingress: "+boolYAML(ent.DirectIngress))
			if ent.Method != "" {
				lines = append(lines, "    method: \""+yamlQuote(ent.Method)+"\"")
			}
		}
	}
	_ = fia
	return strings.Join(lines, "\n") + "\n"
}

func keysOfEdgeClusters(m map[string]AnsibleEdgeCluster) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func BuildInventoryINI() string {
	return "[hub]\nlocalhost ansible_connection=local\n"
}

func EnsureInventoryCM(ctx context.Context, c client.Client, ns string, desired DesiredState, cfg config.Config, fia *fleetv1alpha1.FleetIngressAuth) error {
	edgeClusters, err := LoadEdgeClustersForAnsible(ctx, c, cfg.EdgeClustersCMNamespace, cfg.EdgeClustersCMName)
	if err != nil {
		return err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      InventoryCMName,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name": "fleet-ingress-auth",
			},
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, c, cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data["inventory.ini"] = BuildInventoryINI()
		data := BuildInventoryYAML(cfg, desired, fia, edgeClusters)
		if wanLines := hubwan.InventoryYAMLLines(hubwan.Resolve(ctx, c, cfg)); len(wanLines) > 0 {
			data += strings.Join(wanLines, "\n") + "\n"
		}
		data += strings.Join(hubacme.InventoryYAMLLines(hubacme.Detect(ctx, c)), "\n") + "\n"
		cm.Data["ingress_auth_vars.yml"] = data
		return nil
	})
	return err
}

func boolYAML(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func yamlQuote(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

func yamlStringList(key string, vals []string) []string {
	if len(vals) == 0 {
		return []string{key + ": []"}
	}
	out := []string{key + ":"}
	for _, v := range vals {
		out = append(out, "  - "+v)
	}
	return out
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func sliceToSet(vals []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, v := range vals {
		out[v] = struct{}{}
	}
	return out
}

type edgeClusterEntry struct {
	Management bool `yaml:"management,omitempty"`
	ReadOnly   bool `yaml:"read_only,omitempty"`
}

type edgeClustersFile struct {
	Clusters map[string]edgeClusterEntry `yaml:"clusters"`
}

func loadEdgeClusters(ctx context.Context, c client.Client, ns, name string) (edgeClustersFile, error) {
	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return edgeClustersFile{Clusters: map[string]edgeClusterEntry{}}, nil
		}
		return edgeClustersFile{}, err
	}
	raw := cm.Data["clusters.yaml"]
	if raw == "" {
		return edgeClustersFile{Clusters: map[string]edgeClusterEntry{}}, nil
	}
	var f edgeClustersFile
	if err := yamlUnmarshal(raw, &f); err != nil {
		return edgeClustersFile{}, err
	}
	if f.Clusters == nil {
		f.Clusters = map[string]edgeClusterEntry{}
	}
	return f, nil
}

func yamlUnmarshal(raw string, out interface{}) error {
	return yaml.Unmarshal([]byte(raw), out)
}
