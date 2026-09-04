package ingress

import (
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

// ClusterIngress is the resolved WAN ingress path for a cluster bound to a site ingress entry.
type ClusterIngress struct {
	IngressID       string
	Method          string
	Address         string
	Zone            string
	Wildcard        string
	TlsPolicy       string
	AllowsAuth      bool
	AllowsSpokeACME bool
	AllowsSiteRoute bool
}

// ResolveClusterIngress returns ingress metadata for a cluster bound to ingressID on the site.
func ResolveClusterIngress(site fleetv1alpha1.FleetSiteSpec, ingressID string) (ClusterIngress, error) {
	ingressID = strings.TrimSpace(ingressID)
	if ingressID == "" {
		return ClusterIngress{}, fmt.Errorf("ingress id is required")
	}
	site.NormalizeIngress()
	ent, ok := site.IngressByID(ingressID)
	if !ok {
		return ClusterIngress{}, fmt.Errorf("ingress %q not found on site", ingressID)
	}
	out := ClusterIngress{
		IngressID: ingressID,
		Method:    ent.Method,
		Address:   strings.TrimSpace(ent.Address),
		Zone:      strings.TrimSpace(ent.Zone),
		Wildcard:  strings.TrimSpace(ent.EffectiveWildcard()),
	}
	switch ent.Method {
	case fleetv1alpha1.IngressMethodPortForward:
		if out.Address == "" {
			return ClusterIngress{}, fmt.Errorf("ingress %q: address is required for port_forward", ingressID)
		}
		if out.Zone == "" {
			return ClusterIngress{}, fmt.Errorf("ingress %q: zone is required for port_forward", ingressID)
		}
		out.TlsPolicy = fleetv1alpha1.TLSPolicyACMECluster
		out.AllowsAuth = true
		out.AllowsSpokeACME = true
		out.AllowsSiteRoute = true
	case fleetv1alpha1.IngressMethodReverseProxy:
		out.TlsPolicy = fleetv1alpha1.TLSPolicyExternal
		out.AllowsSiteRoute = true
	default:
		return ClusterIngress{}, fmt.Errorf("ingress %q: unsupported method %q", ingressID, ent.Method)
	}
	return out, nil
}

// ValidateBinding enforces cardinality: port_forward allows one cluster per ingress id.
func ValidateBinding(site fleetv1alpha1.FleetSiteSpec, ingressID, clusterName string, existing map[string][]string) error {
	ingressID = strings.TrimSpace(ingressID)
	if ingressID == "" {
		return fmt.Errorf("ingress id is required")
	}
	if _, err := ResolveClusterIngress(site, ingressID); err != nil {
		return err
	}
	ent, _ := site.IngressByID(ingressID)
	if ent.Method != fleetv1alpha1.IngressMethodPortForward {
		return nil
	}
	for _, name := range existing[ingressID] {
		if name != clusterName && name != "" {
			return fmt.Errorf("ingress %q (port_forward) already bound to cluster %q", ingressID, name)
		}
	}
	return nil
}

// HostMatchesWildcard reports whether host matches a wildcard like *.zone.example.com.
func HostMatchesWildcard(host, wildcard string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	wildcard = strings.TrimSpace(wildcard)
	if host == "" || wildcard == "" {
		return false
	}
	if !strings.HasPrefix(wildcard, "*.") {
		return strings.EqualFold(host, wildcard)
	}
	suffix := strings.ToLower(strings.TrimPrefix(wildcard, "*."))
	if host == suffix {
		return true
	}
	return strings.HasSuffix(host, "."+suffix)
}

// DefaultSiteHost returns default site route hostname for port_forward ingress.
func DefaultSiteHost(release string, ci ClusterIngress) string {
	release = strings.TrimSpace(release)
	if release == "" || ci.Zone == "" || ci.Method != fleetv1alpha1.IngressMethodPortForward {
		return ""
	}
	return release + "." + ci.Zone
}
