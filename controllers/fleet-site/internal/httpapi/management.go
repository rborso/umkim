package httpapi

import (
	"context"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/registry"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (s *Server) managementSiteID() string {
	if s.Cfg.ManagementSiteID != "" {
		return s.Cfg.ManagementSiteID
	}
	return "umkim"
}

func (s *Server) clusterReadOnly(name string, edge edgeClustersFile) bool {
	if e, ok := edge.Clusters[name]; ok && (e.ReadOnly || e.Management) {
		return true
	}
	return false
}

func (s *Server) domainRegistryConfig() registry.DomainConfig {
	return registry.DomainConfig{
		FleetSiteNamespace:       s.Cfg.Namespace,
		EdgeDomainsCMNamespace:   s.Cfg.EdgeDomainsCMNamespace,
		EdgeDomainsCMName:        s.Cfg.EdgeDomainsCMName,
		EdgeClustersCMNamespace:  s.Cfg.EdgeClustersCMNamespace,
		EdgeClustersCMName:       s.Cfg.EdgeClustersCMName,
		HubEdgeDomain:            s.Cfg.HubEdgeDomain,
		ManagementSiteID:         s.managementSiteID(),
	}
}

func (s *Server) siteIsManagement(ctx context.Context, name string) bool {
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &fs); err != nil {
		if apierrors.IsNotFound(err) {
			return name == s.managementSiteID()
		}
		return false
	}
	return isManagementFleetSite(&fs, fs.Spec.EffectiveSiteID(fs.Name), s.managementSiteID())
}

func isManagementFleetSite(fs *fleetv1alpha1.FleetSite, siteID, managementSiteID string) bool {
	if siteID == managementSiteID && managementSiteID != "" {
		return true
	}
	if fs.Annotations != nil && fs.Annotations[registry.AnnotationManagementSite] == "true" {
		return true
	}
	return false
}
