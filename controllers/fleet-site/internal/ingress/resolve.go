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
	WanAddressMode  string
	Zone            string
	Wildcard        string
	TlsPolicy       string
	AllowsAuth      bool
	AllowsSpokeACME bool
	AllowsSiteRoute bool
}

// EdgeWANIP returns the fleet-edge-clusters edge_wan_ip value for resolved ingress.
func EdgeWANIP(ci ClusterIngress) string {
	if ci.WanAddressMode == fleetv1alpha1.WanAddressModeDNS {
		return ""
	}
	return ci.Address
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
	out.WanAddressMode = site.EffectiveWanAddressMode(ent)
	switch ent.Method {
	case fleetv1alpha1.IngressMethodPortForward:
		if out.Address == "" && out.WanAddressMode == fleetv1alpha1.WanAddressModeFixed {
			return ClusterIngress{}, fmt.Errorf("ingress %q: address is required for port_forward with wanAddressMode fixed", ingressID)
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
		// TLS terminates on the operator's proxy, but Authelia forward-auth can still
		// run on the spoke Traefik. A zone (external domain) is required to derive the
		// portal host, cookie domain, and access domains.
		if out.Zone != "" {
			out.AllowsAuth = true
		}
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

// ValidateRouteHost rejects a public route host that is not covered by the
// resolved ingress wildcard. The wildcard is what the operator's DNS and router
// are configured for, so a host outside it would never resolve publicly. When
// the ingress has no wildcard (for example a reverse_proxy entry with no zone)
// there is nothing to enforce and the host is accepted.
func ValidateRouteHost(ci ClusterIngress, host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	wildcard := strings.TrimSpace(ci.Wildcard)
	if wildcard == "" {
		return nil
	}
	if !HostMatchesWildcard(host, wildcard) {
		return fmt.Errorf("host %q is outside the %s ingress %q wildcard %q",
			host, ci.Method, ci.IngressID, wildcard)
	}
	return nil
}
