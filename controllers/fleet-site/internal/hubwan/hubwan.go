package hubwan

import (
	"context"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const fleetEdgeControllerNS = "fleet-system"

func Invalid(ip string) bool {
	ip = strings.TrimSpace(strings.ToLower(ip))
	return ip == "" || ip == "127.0.0.1" || ip == "localhost"
}

// Resolve returns the hub WAN IPv4 for console Ansible Jobs (ACME, ingress auth).
func Resolve(ctx context.Context, c client.Client, cfg config.Config) string {
	var svc corev1.Service
	if err := c.Get(ctx, types.NamespacedName{Namespace: "dcm", Name: "gateway"}, &svc); err == nil {
		if len(svc.Spec.ExternalIPs) > 0 {
			if ip := strings.TrimSpace(svc.Spec.ExternalIPs[0]); !Invalid(ip) {
				return ip
			}
		}
	}
	if ip := strings.TrimSpace(cfg.HubPublicIPv4); !Invalid(ip) {
		return ip
	}
	var dep appsv1.Deployment
	if err := c.Get(ctx, types.NamespacedName{Namespace: fleetEdgeControllerNS, Name: "fleet-edge-controller"}, &dep); err == nil {
		for _, container := range dep.Spec.Template.Spec.Containers {
			if container.Name != "controller" {
				continue
			}
			for _, env := range container.Env {
				switch env.Name {
				case "FLEET_GATEWAY_WAN_IP", "FLEET_GATEWAY_PROBE_HOST":
					if v := strings.TrimSpace(env.Value); !Invalid(v) {
						return v
					}
				}
			}
		}
	}
	return ""
}

func yamlQuote(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// InventoryYAMLLines emits Ansible vars for fleet-edge-controller WAN probe/DNS.
func InventoryYAMLLines(wan string) []string {
	wan = strings.TrimSpace(wan)
	if Invalid(wan) {
		return nil
	}
	return []string{
		"fleet_hub_public_ipv4: \"" + yamlQuote(wan) + "\"",
		"dcm_gateway_external_ip: \"" + yamlQuote(wan) + "\"",
		"fleet_hub_firewall_public_ipv4: \"" + yamlQuote(wan) + "\"",
		"fleet_gateway_probe_host: \"" + yamlQuote(wan) + "\"",
	}
}
