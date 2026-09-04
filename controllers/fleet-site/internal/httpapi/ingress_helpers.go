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

func validateSiteIngressRows(ingress []fleetv1alpha1.FleetSiteIngressSpec) error {
	seen := map[string]struct{}{}
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
			if strings.TrimSpace(ent.Address) == "" {
				return fmt.Errorf("ingress %q: address is required for port_forward", id)
			}
			if strings.TrimSpace(ent.Zone) == "" {
				return fmt.Errorf("ingress %q: zone is required for port_forward", id)
			}
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
		IngressID:     ci.IngressID,
		Method:        ci.Method,
		TlsPolicy:     ci.TlsPolicy,
		DirectIngress: registry.DirectIngressFromMethod(ci.Method),
		DNSZone:       ci.Zone,
		EdgeWANIP:     ci.Address,
	}, nil
}

func clusterIngressAllowsAuth(site *fleetv1alpha1.FleetSite, ingressID string) bool {
	if site == nil || ingressID == "" {
		return false
	}
	ci, err := ingress.ResolveClusterIngress(site.Spec, ingressID)
	return err == nil && ci.AllowsAuth
}

func writeIngressConflict(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	http.Error(w, err.Error(), http.StatusConflict)
}
