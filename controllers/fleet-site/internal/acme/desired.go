package acme

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/hubwan"
	"github.com/rborso/umkim/controllers/fleet-site/internal/hubacme"
	"github.com/rborso/umkim/controllers/fleet-site/internal/teardown"
	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	ReconcileAnnotation       = "fleet.umkim.com/acme-reconcile"
	AnsibleSpecHashAnnotation = "fleet.umkim.com/acme-spec-hash"
	InventoryCMName           = "fleet-acme-inventory"
)

type edgeClusterEntry struct {
	DirectIngress bool   `yaml:"direct_ingress"`
	Method        string `yaml:"method,omitempty"`
	TlsPolicy     string `yaml:"tls_policy,omitempty"`
	Management    bool   `yaml:"management,omitempty"`
	ReadOnly      bool   `yaml:"read_only,omitempty"`
}

type edgeClustersFile struct {
	Clusters map[string]edgeClusterEntry `yaml:"clusters"`
}

func loadEdgeClusters(ctx context.Context, c client.Client, ns, name string) (edgeClustersFile, error) {
	if ns == "" || name == "" {
		return edgeClustersFile{Clusters: map[string]edgeClusterEntry{}}, nil
	}
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
	if err := yaml.Unmarshal([]byte(raw), &f); err != nil {
		return edgeClustersFile{}, err
	}
	if f.Clusters == nil {
		f.Clusters = map[string]edgeClusterEntry{}
	}
	return f, nil
}

type DesiredState struct {
	HubEnabled       bool
	Staging          bool
	Email            string
	EnabledClusters  []string
	Phase6Clusters   []string
	GlobalEnabled    bool
}

func EnsureCR(ctx context.Context, c client.Client, ns string, cfg config.Config) (*fleetv1alpha1.FleetAcme, error) {
	var fa fleetv1alpha1.FleetAcme
	err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: fleetv1alpha1.FleetAcmeName}, &fa)
	if err == nil {
		return &fa, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}
	fa = fleetv1alpha1.FleetAcme{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fleetv1alpha1.FleetAcmeName,
			Namespace: ns,
		},
		Status: fleetv1alpha1.FleetAcmeStatus{
			Phase: fleetv1alpha1.AcmePhaseDisabled,
		},
	}
	if err := c.Create(ctx, &fa); err != nil {
		return nil, err
	}
	return &fa, nil
}

func LegacyHubACMEEnabled(ctx context.Context, c client.Client) (bool, error) {
	var cm corev1.ConfigMap
	err := c.Get(ctx, types.NamespacedName{Namespace: "fleet-ingress", Name: "fleet-traefik-static"}, &cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return strings.Contains(cm.Data["traefik.yml"], "letsencrypt"), nil
}

func ComputeDesired(ctx context.Context, c client.Client, cfg config.Config, fa *fleetv1alpha1.FleetAcme) (DesiredState, error) {
	out := DesiredState{
		HubEnabled: fa.Spec.HubEnabled,
		Staging:    fa.Spec.Staging,
		Email:      fa.Spec.Email,
	}
	if out.Email == "" {
		out.Email = cfg.FleetEdgeACMEEmail
	}

	edge, err := loadEdgeClusters(ctx, c, cfg.EdgeClustersCMNamespace, cfg.EdgeClustersCMName)
	if err != nil {
		return out, err
	}

	var joins fleetv1alpha1.FleetClusterJoinList
	if err := c.List(ctx, &joins, client.InNamespace(cfg.Namespace)); err != nil {
		return out, err
	}
	enabledSet := map[string]struct{}{}
	for _, fcj := range joins.Items {
		name := fcj.Spec.EffectiveClusterName(fcj.Name)
		if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady || !fcj.Spec.Acme.Enabled {
			continue
		}
		ent, ok := edge.Clusters[name]
		if !ok {
			continue
		}
		tlsPolicy := ent.TlsPolicy
		if tlsPolicy == "" || tlsPolicy == "inherit" {
			tlsPolicy = fleetv1alpha1.TLSPolicyACMECluster
		}
		if tlsPolicy != fleetv1alpha1.TLSPolicyACMECluster {
			continue
		}
		if !ent.DirectIngress {
			continue
		}
		if !teardown.ManagedClusterExists(ctx, c, name) {
			continue
		}
		enabledSet[name] = struct{}{}
		out.EnabledClusters = append(out.EnabledClusters, name)
	}
	out.EnabledClusters = sortedKeys(enabledSet)

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
		if _, ok := enabledSet[name]; ok {
			out.Phase6Clusters = append(out.Phase6Clusters, name)
		}
	}
	out.Phase6Clusters = sortedKeys(sliceToSet(out.Phase6Clusters))

	out.GlobalEnabled = out.HubEnabled || len(out.EnabledClusters) > 0
	return out, nil
}

type HubRunnerVars struct {
	GatewayExternalIP string
	GatewayReplicas   int
}

func hubGatewayRunnerVars(ctx context.Context, c client.Client, cfg config.Config) HubRunnerVars {
	out := HubRunnerVars{GatewayReplicas: 1}
	var svc corev1.Service
	if err := c.Get(ctx, types.NamespacedName{Namespace: "fleet-ingress", Name: "gateway"}, &svc); err == nil {
		if len(svc.Spec.ExternalIPs) > 0 {
			out.GatewayExternalIP = svc.Spec.ExternalIPs[0]
		}
	}
	var dep appsv1.Deployment
	if err := c.Get(ctx, types.NamespacedName{Namespace: "fleet-ingress", Name: "gateway"}, &dep); err == nil {
		if dep.Spec.Replicas != nil && *dep.Spec.Replicas > 0 {
			out.GatewayReplicas = int(*dep.Spec.Replicas)
		}
	}
	if hubwan.Invalid(out.GatewayExternalIP) {
		out.GatewayExternalIP = hubwan.Resolve(ctx, c, cfg)
	}
	return out
}

func BuildInventoryYAML(cfg config.Config, desired DesiredState, fa *fleetv1alpha1.FleetAcme, hub HubRunnerVars, edge edgeClustersFile) string {
	email := desired.Email
	if email == "" {
		email = cfg.FleetEdgeACMEEmail
	}
	gatewayURL := "http://127.0.0.1:80"
	if desired.HubEnabled {
		gatewayURL = "https://127.0.0.1"
	}
	lines := []string{
		"fleet_acme_hub_enabled: " + boolYAML(desired.HubEnabled),
		"fleet_acme_staging: " + boolYAML(desired.Staging),
		"fleet_acme_email: \"" + yamlQuote(email) + "\"",
		"fleet_edge_acme_enabled: " + boolYAML(desired.GlobalEnabled),
		"fleet_edge_acme_staging: " + boolYAML(desired.Staging),
		"fleet_edge_acme_email: \"" + yamlQuote(email) + "\"",
		"fleet_edge_enable: true",
		"fleet_edge_controller_enabled: true",
		"fleet_hub_firewall_enabled: false",
		"fleet_edge_domain: \"" + yamlQuote(cfg.HubEdgeDomain) + "\"",
		"fleet_edge_hub_gateway_url: \"" + yamlQuote(gatewayURL) + "\"",
		"harbor_host: \"" + yamlQuote(cfg.HubWireguardIP) + "\"",
		"harbor_nodeport_https: \"30443\"",
		"harbor_proxy_project_ghcr: ghcr-proxy",
		"ocm_kubeconfig: /etc/kube/config",
		"k3s_command_path: /usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"fleet_edge_controller_playbook_dir: /ansible",
		"hub_ingress_namespace: fleet-ingress",
		"fleet_acme_gateway_replicas: " + fmt.Sprintf("%d", hub.GatewayReplicas),
	}
	lines = append(lines, hubwan.InventoryYAMLLines(hub.GatewayExternalIP)...)
	if len(desired.Phase6Clusters) == 0 {
		lines = append(lines, "ocm_manifestwork_phase6_clusters: []")
	} else {
		lines = append(lines, "ocm_manifestwork_phase6_clusters:")
		for _, name := range desired.Phase6Clusters {
			lines = append(lines, "  - "+yamlQuote(name))
		}
	}
	if len(desired.EnabledClusters) == 0 {
		lines = append(lines, "fleet_acme_spoke_clusters: []")
	} else {
		lines = append(lines, "fleet_acme_spoke_clusters:")
		for _, name := range desired.EnabledClusters {
			lines = append(lines, "  - "+yamlQuote(name))
		}
	}
	if len(desired.Phase6Clusters) > 0 {
		lines = append(lines, "fleet_edge_clusters:")
		for _, name := range desired.Phase6Clusters {
			ent, ok := edge.Clusters[name]
			if !ok {
				continue
			}
			lines = append(lines, "  "+name+":")
			lines = append(lines, "    direct_ingress: "+boolYAML(ent.DirectIngress))
			if ent.Method != "" {
				lines = append(lines, "    method: \""+yamlQuote(ent.Method)+"\"")
			}
			if ent.TlsPolicy != "" {
				lines = append(lines, "    tls_policy: \""+yamlQuote(ent.TlsPolicy)+"\"")
			}
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func boolYAML(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func yamlQuote(s string) string {
	return strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(s)
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

func sliceToSet(items []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, item := range items {
		out[item] = struct{}{}
	}
	return out
}

func EnsureInventoryCM(ctx context.Context, c client.Client, ns string, desired DesiredState, cfg config.Config, fa *fleetv1alpha1.FleetAcme) error {
	hub := hubGatewayRunnerVars(ctx, c, cfg)
	edge, err := loadEdgeClusters(ctx, c, cfg.EdgeClustersCMNamespace, cfg.EdgeClustersCMName)
	if err != nil {
		return err
	}
	data := BuildInventoryYAML(cfg, desired, fa, hub, edge)
	data += strings.Join(hubacme.InventoryYAMLLines(hubacme.Detect(ctx, c)), "\n") + "\n"
	if len(desired.Phase6Clusters) > 0 {
		// Spoke ACME queue needs letsencrypt-staging even when hub gateway is legacy.
		data += "fleet_edge_acme_preflight_enabled: true\n"
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      InventoryCMName,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name": "fleet-acme",
			},
		},
		Data: map[string]string{
			"acme_vars.yml": data,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, c, cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data["inventory.ini"] = "[hub]\nlocalhost ansible_connection=local\n"
		cm.Data["acme_vars.yml"] = data
		return nil
	})
	return err
}

func AnsibleSpecHash(desired DesiredState) string {
	payload := fmt.Sprintf("hub=%v staging=%v email=%s clusters=%v phase6=%v",
		desired.HubEnabled, desired.Staging, desired.Email, desired.EnabledClusters, desired.Phase6Clusters)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:8])
}

func storedAnsibleSpecHash(fa *fleetv1alpha1.FleetAcme) string {
	if fa.Annotations == nil {
		return ""
	}
	return fa.Annotations[AnsibleSpecHashAnnotation]
}

func AnsibleSpecChanged(fa *fleetv1alpha1.FleetAcme, desired DesiredState) bool {
	return storedAnsibleSpecHash(fa) != AnsibleSpecHash(desired)
}
