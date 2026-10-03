package ingress

import (
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

// EdgeClusterIngressFields mirrors fleet-edge-clusters ingress-derived YAML fields.
type EdgeClusterIngressFields struct {
	Method         string
	TlsPolicy      string
	DNSZone        string
	EdgeWANIP      string
	WanAddressMode string
	DirectIngress  bool
}

// ResolveEdgeClusterIngress resolves ingress metadata for fleet-edge-clusters upsert/sync.
func ResolveEdgeClusterIngress(site fleetv1alpha1.FleetSiteSpec, ingressID string) (EdgeClusterIngressFields, error) {
	ci, err := ResolveClusterIngress(site, ingressID)
	if err != nil {
		return EdgeClusterIngressFields{}, err
	}
	direct := ci.Method == fleetv1alpha1.IngressMethodPortForward ||
		ci.Method == fleetv1alpha1.IngressMethodReverseProxy
	return EdgeClusterIngressFields{
		Method:         ci.Method,
		TlsPolicy:      ci.TlsPolicy,
		DNSZone:        ci.Zone,
		EdgeWANIP:      EdgeWANIP(ci),
		WanAddressMode: ci.WanAddressMode,
		DirectIngress:  direct,
	}, nil
}
