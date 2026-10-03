package overlay

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/wg"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const defaultReservedOverlayHosts = "10.10.0.1,10.10.0.2,10.10.0.3,10.10.0.11"

// CollectUsed returns overlay host IPs already reserved or allocated in the fleet-console namespace.
func CollectUsed(ctx context.Context, c client.Client, namespace string, reserved []string) (map[string]bool, error) {
	used := map[string]bool{}
	for _, host := range reserved {
		host = strings.TrimSpace(host)
		if host != "" {
			used[host] = true
		}
	}

	ns := namespace
	var sites fleetv1alpha1.FleetSiteList
	if err := c.List(ctx, &sites, client.InNamespace(ns)); err != nil {
		return nil, err
	}
	for _, s := range sites.Items {
		if s.Status.WireguardAddress != "" {
			MarkUsed(used, s.Status.WireguardAddress)
		}
	}

	var joins fleetv1alpha1.FleetClusterJoinList
	if err := c.List(ctx, &joins, client.InNamespace(ns)); err != nil {
		return nil, err
	}
	for _, j := range joins.Items {
		if j.Status.WireguardAddress != "" {
			MarkUsed(used, j.Status.WireguardAddress)
		}
		markBootstrapNodeOverlayIPs(ctx, c, ns, j.Name, used)
	}
	return used, nil
}

func markBootstrapNodeOverlayIPs(ctx context.Context, c client.Client, ns, clusterName string, used map[string]bool) {
	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: "fleet-cluster-" + clusterName + "-bootstrap"}, &secret); err != nil {
		return
	}
	raw, ok := secret.Data["nodes-wg.json"]
	if !ok {
		return
	}
	var nodes []wg.NodeWG
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return
	}
	for _, n := range nodes {
		if n.WGAddress != "" {
			MarkUsed(used, n.WGAddress)
		}
	}
}

// MarkUsed records a host IP from a CIDR or bare address.
func MarkUsed(used map[string]bool, cidr string) {
	if ip := wg.HostIP(cidr); ip != "" {
		used[ip] = true
	}
}

// AssertAvailable reports whether cidr's host IP is not already used.
func AssertAvailable(used map[string]bool, cidr string) error {
	ip := wg.HostIP(cidr)
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid wireguard overlay address %q", cidr)
	}
	if used[ip] {
		return fmt.Errorf("wireguard overlay IP %s is already allocated or reserved", ip)
	}
	return nil
}

// ParseReservedHosts splits a comma-separated host list, using defaultReservedOverlayHosts when empty.
func ParseReservedHosts(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultReservedOverlayHosts
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
