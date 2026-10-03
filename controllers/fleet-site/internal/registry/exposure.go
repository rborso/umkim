package registry

import fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"

// ClusterIngressInput mirrors fleet-edge-clusters YAML entry fields.
type ClusterIngressInput struct {
	IngressID      string
	Method         string
	TlsPolicy      string
	DirectIngress  bool
	DNSZone        string
	EdgeWANIP      string
	WanAddressMode string
}

// ClusterIngressFromResolved builds registry input from resolved ingress metadata.
func ClusterIngressFromResolved(ci struct {
	IngressID, Method, TlsPolicy, Address, Zone, WanAddressMode string
}) ClusterIngressInput {
	edgeWAN := ci.Address
	if ci.WanAddressMode == fleetv1alpha1.WanAddressModeDNS {
		edgeWAN = ""
	}
	return ClusterIngressInput{
		IngressID:      ci.IngressID,
		Method:         ci.Method,
		TlsPolicy:      ci.TlsPolicy,
		DirectIngress:  AllowsDirectSiteIngress(ci.Method),
		DNSZone:        ci.Zone,
		EdgeWANIP:      edgeWAN,
		WanAddressMode: ci.WanAddressMode,
	}
}

// AllowsDirectSiteIngress is true for port_forward and reverse_proxy (WAN reaches cluster Traefik).
func AllowsDirectSiteIngress(method string) bool {
	return method == fleetv1alpha1.IngressMethodPortForward ||
		method == fleetv1alpha1.IngressMethodReverseProxy
}

// DirectIngressFromMethod maps ingress method to fleet-edge direct_ingress flag.
func DirectIngressFromMethod(method string) bool {
	return AllowsDirectSiteIngress(method)
}

// SpokeACMEEnabled reports whether spoke Traefik should run Let's Encrypt.
func SpokeACMEEnabled(tlsPolicy string) bool {
	return tlsPolicy == fleetv1alpha1.TLSPolicyACMECluster
}

// Legacy aliases for gradual call-site migration.
type ClusterExposureInput = ClusterIngressInput

func EffectiveClusterExposureMode(input ClusterExposureInput, _ string) string {
	if input.Method != "" {
		return input.Method
	}
	if input.DirectIngress {
		return fleetv1alpha1.IngressMethodPortForward
	}
	return fleetv1alpha1.ExposureModeNone
}

func DirectIngressFromExposure(mode string) bool {
	return AllowsDirectSiteIngress(mode)
}

func EffectiveClusterTLSPolicy(input ClusterExposureInput, _ string) string {
	if input.TlsPolicy != "" {
		return input.TlsPolicy
	}
	switch input.Method {
	case fleetv1alpha1.IngressMethodReverseProxy:
		return fleetv1alpha1.TLSPolicyExternal
	case fleetv1alpha1.IngressMethodPortForward:
		return fleetv1alpha1.TLSPolicyACMECluster
	default:
		return fleetv1alpha1.TLSPolicyInherit
	}
}
