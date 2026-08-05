package registry

import fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"

// ClusterExposureInput mirrors fleet-edge-clusters YAML entry fields.
type ClusterExposureInput struct {
	ExposureMode      string
	TlsPolicy         string
	DirectIngress     bool
	IngressEndpointID string
	DomainBindings    []string
}

// EffectiveClusterExposureMode resolves cluster override vs site default vs legacy direct_ingress.
func EffectiveClusterExposureMode(cluster ClusterExposureInput, siteMode string) string {
	if cluster.ExposureMode != "" && cluster.ExposureMode != "inherit" {
		return cluster.ExposureMode
	}
	if siteMode != "" {
		return siteMode
	}
	if cluster.DirectIngress {
		return fleetv1alpha1.ExposureModePortForward
	}
	return fleetv1alpha1.ExposureModeNone
}

// AllowsDirectSiteIngress is true for port_forward and reverse_proxy (WAN reaches cluster Traefik).
func AllowsDirectSiteIngress(mode string) bool {
	return mode == fleetv1alpha1.ExposureModePortForward || mode == fleetv1alpha1.ExposureModeReverseProxy
}

// DirectIngressFromExposure maps exposure mode to fleet-edge direct_ingress flag.
func DirectIngressFromExposure(mode string) bool {
	return AllowsDirectSiteIngress(mode)
}

// EffectiveClusterTLSPolicy resolves TLS policy for a cluster.
func EffectiveClusterTLSPolicy(cluster ClusterExposureInput, siteMode string) string {
	if cluster.TlsPolicy != "" && cluster.TlsPolicy != fleetv1alpha1.TLSPolicyInherit {
		return cluster.TlsPolicy
	}
	mode := EffectiveClusterExposureMode(cluster, siteMode)
	switch mode {
	case fleetv1alpha1.ExposureModeReverseProxy:
		return fleetv1alpha1.TLSPolicyExternal
	case fleetv1alpha1.ExposureModePortForward:
		return fleetv1alpha1.TLSPolicyACMECluster
	case fleetv1alpha1.ExposureModeNone:
		return fleetv1alpha1.TLSPolicyACMEHub
	default:
		return fleetv1alpha1.TLSPolicyInherit
	}
}

// SpokeACMEEnabled reports whether spoke Traefik should run Let's Encrypt.
func SpokeACMEEnabled(tlsPolicy string) bool {
	return tlsPolicy == fleetv1alpha1.TLSPolicyACMECluster
}
