package reconcile

import (
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/registry"
)

// edgeClusterUpsert mirrors fleet-edge-clusters YAML fields written by FCJ reconcile.
type edgeClusterUpsert struct {
	Site              string
	Management        bool
	ReadOnly          bool
	DirectIngress     bool
	ExposureMode      string
	TlsPolicy         string
	IngressEndpointID string
	DomainBindings    []string
	UpstreamHost      string
	TraefikHTTPPort   int
}

func mergeEdgeClusterEntryForJoin(prev edgeClusterUpsert, fcj *fleetv1alpha1.FleetClusterJoin, siteDefaultExposure, upstreamHost string) edgeClusterUpsert {
	exposure := fcj.Spec.Edge.ExposureMode
	if exposure == "" {
		exposure = "inherit"
	}
	tls := fcj.Spec.Edge.TlsPolicy
	if tls == "" {
		tls = "inherit"
	}
	clusterInput := registry.ClusterExposureInput{
		ExposureMode:   exposure,
		TlsPolicy:      tls,
		DomainBindings: fcj.Spec.DomainBindings,
	}
	effectiveExposure := registry.EffectiveClusterExposureMode(clusterInput, siteDefaultExposure)

	domainBindings := append([]string(nil), fcj.Spec.DomainBindings...)
	if len(domainBindings) == 0 {
		domainBindings = append([]string(nil), prev.DomainBindings...)
	}

	ingressEndpointID := strings.TrimSpace(fcj.Spec.Edge.IngressEndpointId)
	if ingressEndpointID == "" {
		ingressEndpointID = prev.IngressEndpointID
	}

	traefikHTTPPort := prev.TraefikHTTPPort
	if traefikHTTPPort == 0 {
		traefikHTTPPort = 30080
	}

	return edgeClusterUpsert{
		Site:              fcj.Spec.SiteId,
		Management:        prev.Management,
		ReadOnly:          prev.ReadOnly,
		DirectIngress:     registry.DirectIngressFromExposure(effectiveExposure),
		ExposureMode:      effectiveExposure,
		TlsPolicy:         registry.EffectiveClusterTLSPolicy(clusterInput, siteDefaultExposure),
		IngressEndpointID: ingressEndpointID,
		DomainBindings:    domainBindings,
		UpstreamHost:      upstreamHost,
		TraefikHTTPPort:   traefikHTTPPort,
	}
}
