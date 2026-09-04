package reconcile

import (
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingress"
	"github.com/rborso/umkim/controllers/fleet-site/internal/registry"
)

// edgeClusterUpsert mirrors fleet-edge-clusters YAML fields written by FCJ reconcile.
type edgeClusterUpsert struct {
	Site          string
	Management    bool
	ReadOnly      bool
	DirectIngress bool
	IngressID     string
	Method        string
	TlsPolicy     string
	DNSZone       string
	EdgeWANIP     string
	UpstreamHost  string
	TraefikHTTPPort int
}

func mergeEdgeClusterEntryForJoin(prev edgeClusterUpsert, fcj *fleetv1alpha1.FleetClusterJoin, site *fleetv1alpha1.FleetSite, upstreamHost string) edgeClusterUpsert {
	ingressID := fcj.Spec.Edge.IngressId
	if ingressID == "" {
		ingressID = prev.IngressID
	}
	if ingressID == "" && site != nil {
		ingressID = site.Spec.AutoIngressID()
	}

	out := edgeClusterUpsert{
		Site:            fcj.Spec.SiteId,
		Management:      prev.Management,
		ReadOnly:        prev.ReadOnly,
		UpstreamHost:    upstreamHost,
		TraefikHTTPPort: prev.TraefikHTTPPort,
		IngressID:       ingressID,
	}
	if out.TraefikHTTPPort == 0 {
		out.TraefikHTTPPort = 30080
	}

	if site != nil && ingressID != "" {
		if ci, err := ingress.ResolveClusterIngress(site.Spec, ingressID); err == nil {
			out.Method = ci.Method
			out.TlsPolicy = ci.TlsPolicy
			out.DNSZone = ci.Zone
			out.EdgeWANIP = ci.Address
			out.DirectIngress = registry.DirectIngressFromMethod(ci.Method)
		}
	}
	if out.Method == "" {
		out.DirectIngress = prev.DirectIngress
		out.Method = prev.Method
		out.TlsPolicy = prev.TlsPolicy
		out.DNSZone = prev.DNSZone
		out.EdgeWANIP = prev.EdgeWANIP
	}
	return out
}
