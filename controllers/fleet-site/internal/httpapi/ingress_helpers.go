package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingress"
	"github.com/rborso/umkim/controllers/fleet-site/internal/registry"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func validateSiteIngressRows(edge fleetv1alpha1.FleetSiteEdgeSpec, ingress []fleetv1alpha1.FleetSiteIngressSpec) error {
	siteSpec := fleetv1alpha1.FleetSiteSpec{Edge: edge}
	seen := map[string]struct{}{}
	// wildcardOwner maps an effective port_forward wildcard to the id that
	// claimed it. Two port_forward entries claiming the same public hostnames
	// are always a misconfiguration: DNS and the router can only point one way,
	// so one of the entries could never take effect.
	wildcardOwner := map[string]string{}
	for _, ent := range ingress {
		id := strings.TrimSpace(ent.ID)
		if id == "" {
			return fmt.Errorf("each ingress row requires id")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate ingress id %q", id)
		}
		seen[id] = struct{}{}
		switch ent.Method {
		case fleetv1alpha1.IngressMethodPortForward:
			mode := siteSpec.EffectiveWanAddressMode(ent)
			if strings.TrimSpace(ent.Address) == "" && mode == fleetv1alpha1.WanAddressModeFixed {
				return fmt.Errorf("ingress %q: address is required for port_forward with wanAddressMode fixed", id)
			}
			if strings.TrimSpace(ent.Zone) == "" {
				return fmt.Errorf("ingress %q: zone is required for port_forward", id)
			}
			if mode := strings.TrimSpace(ent.WanAddressMode); mode != "" &&
				mode != fleetv1alpha1.WanAddressModeFixed && mode != fleetv1alpha1.WanAddressModeDNS {
				return fmt.Errorf("ingress %q: wanAddressMode must be fixed or dns", id)
			}
			wildcard := strings.ToLower(strings.TrimSpace(ent.EffectiveWildcard()))
			if wildcard == "" {
				break
			}
			if owner, ok := wildcardOwner[wildcard]; ok {
				return fmt.Errorf("ingress %q duplicates wildcard %q already used by ingress %q; two port_forward entries must not claim the same public hostnames",
					id, wildcard, owner)
			}
			wildcardOwner[wildcard] = id
		case fleetv1alpha1.IngressMethodReverseProxy:
			// id only; address and operatorNotes optional
		default:
			return fmt.Errorf("ingress %q: method must be port_forward or reverse_proxy", id)
		}
	}
	return nil
}

func (s *Server) loadSiteForCluster(ctx context.Context, siteID string) (*fleetv1alpha1.FleetSite, error) {
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: s.Cfg.Namespace, Name: siteID}, &fs); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("site %q not found", siteID)
		}
		return nil, err
	}
	fs.Spec.NormalizeIngress()
	return &fs, nil
}

func (s *Server) existingIngressBindings(ctx context.Context, siteID, skipCluster string) (map[string][]string, error) {
	edge, err := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for name, ent := range edge.Clusters {
		if name == skipCluster || ent.IngressID == "" {
			continue
		}
		if ent.Site != "" && ent.Site != siteID {
			continue
		}
		out[ent.IngressID] = append(out[ent.IngressID], name)
	}
	return out, nil
}

func (s *Server) validateClusterIngressBinding(ctx context.Context, siteID, clusterName, ingressID string) error {
	ingressID = strings.TrimSpace(ingressID)
	if ingressID == "" {
		return fmt.Errorf("ingress id is required")
	}
	site, err := s.loadSiteForCluster(ctx, siteID)
	if err != nil {
		return err
	}
	bindings, err := s.existingIngressBindings(ctx, siteID, clusterName)
	if err != nil {
		return err
	}
	return ingress.ValidateBinding(site.Spec, ingressID, clusterName, bindings)
}

func resolveEdgeFromIngress(site *fleetv1alpha1.FleetSite, ingressID string) (registry.ClusterIngressInput, error) {
	ci, err := ingress.ResolveClusterIngress(site.Spec, ingressID)
	if err != nil {
		return registry.ClusterIngressInput{}, err
	}
	return registry.ClusterIngressInput{
		IngressID:      ci.IngressID,
		Method:         ci.Method,
		TlsPolicy:      ci.TlsPolicy,
		DirectIngress:  registry.DirectIngressFromMethod(ci.Method),
		DNSZone:        ci.Zone,
		EdgeWANIP:      ingress.EdgeWANIP(ci),
		WanAddressMode: ci.WanAddressMode,
	}, nil
}

func clusterIngressAllowsAuth(site *fleetv1alpha1.FleetSite, ingressID string) bool {
	if site == nil || ingressID == "" {
		return false
	}
	ci, err := ingress.ResolveClusterIngress(site.Spec, ingressID)
	return err == nil && ci.AllowsAuth
}

// validateAppRouteHosts rejects site/hub route hosts that fall outside the
// wildcard the cluster's bound ingress (site routes) or the site hub gateway
// (hub routes) is configured for. A host outside the wildcard never resolves
// publicly, so accepting it would silently publish a dead URL.
func (s *Server) validateAppRouteHosts(ctx context.Context, clusterName string, routes []fleetv1alpha1.FleetAppRouteIntent) string {
	clusterName = strings.TrimSpace(clusterName)
	if clusterName == "" || len(routes) == 0 {
		return ""
	}
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil || fcj == nil {
		return ""
	}
	siteID := strings.TrimSpace(fcj.Spec.SiteId)
	if siteID == "" {
		return ""
	}
	site, err := s.loadSiteForCluster(ctx, siteID)
	if err != nil || site == nil {
		return ""
	}
	ingressID := strings.TrimSpace(fcj.Spec.Edge.IngressId)
	if ingressID == "" {
		ingressID = site.Spec.AutoIngressID()
	}
	ci := ingress.ClusterIngress{}
	haveIngress := false
	if ingressID != "" {
		if resolved, rerr := ingress.ResolveClusterIngress(site.Spec, ingressID); rerr == nil {
			ci = resolved
			haveIngress = true
		}
	}
	hubWildcard := strings.TrimSpace(site.Spec.EffectiveHubWildcard())
	for _, rt := range routes {
		switch rt.EffectiveMode() {
		case fleetv1alpha1.AppRouteModeSite:
			if !haveIngress {
				continue
			}
			if err := ingress.ValidateRouteHost(ci, rt.SiteHost); err != nil {
				return err.Error()
			}
		case fleetv1alpha1.AppRouteModeHub:
			host := strings.TrimSpace(rt.HubHost)
			if hubWildcard == "" || host == "" {
				continue
			}
			if !ingress.HostMatchesWildcard(host, hubWildcard) {
				return fmt.Sprintf("route hubHost %q is outside the site hub gateway wildcard %q", host, hubWildcard)
			}
		}
	}
	return ""
}

func writeIngressConflict(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	http.Error(w, err.Error(), http.StatusConflict)
}
