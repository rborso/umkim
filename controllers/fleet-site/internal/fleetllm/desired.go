package fleetllm

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	ReconcileAnnotation = "fleet.umkim.com/llm-reconcile"
	InventoryCMName     = "fleet-llm-inventory"
)

type DesiredState struct {
	HubEnabled    bool
	PublicHost    string
	Backends      []fleetv1alpha1.FleetLLMBackend
	GlobalEnabled bool
}

func EnsureCR(ctx context.Context, c client.Client, ns string, _ config.Config) (*fleetv1alpha1.FleetLLM, error) {
	var fl fleetv1alpha1.FleetLLM
	err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: fleetv1alpha1.FleetLLMName}, &fl)
	if err == nil {
		return &fl, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}
	fl = fleetv1alpha1.FleetLLM{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fleetv1alpha1.FleetLLMName,
			Namespace: ns,
		},
		Status: fleetv1alpha1.FleetLLMStatus{
			Phase: fleetv1alpha1.LLMPhaseDisabled,
		},
	}
	if err := c.Create(ctx, &fl); err != nil {
		return nil, err
	}
	return &fl, nil
}

func ComputeDesired(_ context.Context, cfg config.Config, fl *fleetv1alpha1.FleetLLM) DesiredState {
	out := DesiredState{
		HubEnabled: fl.Spec.HubEnabled,
		PublicHost: fl.Spec.EffectivePublicHost(cfg.HubEdgeDomain),
		Backends:   fl.Spec.Backends,
	}
	out.GlobalEnabled = out.HubEnabled
	return out
}

func BuildInventoryYAML(cfg config.Config, desired DesiredState, fl *fleetv1alpha1.FleetLLM) string {
	lines := []string{
		"fleet_llm_hub_enabled: " + boolYAML(desired.HubEnabled),
		"fleet_llm_public_host: \"" + yamlQuote(desired.PublicHost) + "\"",
		"fleet_llm_master_key_secret: \"" + yamlQuote(fl.Spec.EffectiveMasterKeySecretRef()) + "\"",
		"fleet_edge_domain: \"" + yamlQuote(cfg.HubEdgeDomain) + "\"",
		"harbor_host: \"" + yamlQuote(cfg.HubWireguardIP) + "\"",
		"harbor_nodeport_https: \"30443\"",
		"ocm_kubeconfig: /etc/kube/config",
		"k3s_command_path: /usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"phase5_dcm_namespace: dcm",
		"fleet_edge_acme_enabled: " + boolYAML(cfg.FleetEdgeACMEEnabled),
		"fleet_console_namespace: \"" + yamlQuote(cfg.ConsoleNamespace) + "\"",
	}
	if len(desired.Backends) == 0 {
		lines = append(lines, "fleet_llm_backends: []")
	} else {
		lines = append(lines, "fleet_llm_backends:")
		for _, b := range desired.Backends {
			lines = append(lines, "  - model: \""+yamlQuote(b.Model)+"\"")
			lines = append(lines, "    api_base: \""+yamlQuote(b.APIBase)+"\"")
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func EnsureInventoryCM(ctx context.Context, c client.Client, ns string, desired DesiredState, cfg config.Config, fl *fleetv1alpha1.FleetLLM) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      InventoryCMName,
			Namespace: ns,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, c, cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data["llm_vars.yml"] = BuildInventoryYAML(cfg, desired, fl)
		return nil
	})
	return err
}

func HubEndpoint(publicHost string) string {
	if publicHost == "" {
		return ""
	}
	return fmt.Sprintf("https://%s/v1", publicHost)
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
