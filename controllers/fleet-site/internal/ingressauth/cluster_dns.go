package ingressauth

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClusterDNSConfig struct {
	PrimaryZone     string
	PortalHost      string
	LLDAPWebUIHost  string
	AccessDomains   []string
}

func PortalHost(zone string) string {
	return "auth." + strings.TrimPrefix(zone, ".")
}

func LLDAPWebUIHost(zone string) string {
	return "lldap." + strings.TrimPrefix(zone, ".")
}

func AccessDomainsForZones(zones []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, zone := range zones {
		zone = strings.TrimSpace(zone)
		if zone == "" {
			continue
		}
		for _, d := range []string{"*." + zone, zone} {
			if _, ok := seen[d]; ok {
				continue
			}
			seen[d] = struct{}{}
			out = append(out, d)
		}
	}
	return out
}

type edgeClusterDNS struct {
	Site           string   `yaml:"site"`
	DomainBindings []string `yaml:"domain_bindings,omitempty"`
}

type edgeClustersDNSFile struct {
	Clusters map[string]edgeClusterDNS `yaml:"clusters"`
}

func ResolveClusterDNS(ctx context.Context, c client.Client, cfg config.Config, clusterName string, fcj *fleetv1alpha1.FleetClusterJoin) (ClusterDNSConfig, error) {
	bindingIDs := fcj.Spec.DomainBindings
	if len(bindingIDs) == 0 {
		edge, err := loadEdgeClustersDNS(ctx, c, cfg.EdgeClustersCMNamespace, cfg.EdgeClustersCMName)
		if err != nil {
			return ClusterDNSConfig{}, err
		}
		if ent, ok := edge.Clusters[clusterName]; ok {
			bindingIDs = ent.DomainBindings
		}
	}
	if len(bindingIDs) == 0 {
		return ClusterDNSConfig{}, fmt.Errorf("cluster has no domain bindings; configure site DNS zone bindings before enabling ingress auth")
	}

	site, err := loadFleetSiteForJoin(ctx, c, cfg.Namespace, fcj)
	if err != nil {
		return ClusterDNSConfig{}, err
	}

	zones := matchedZones(site, bindingIDs)
	if len(zones) == 0 {
		return ClusterDNSConfig{}, fmt.Errorf("no site DNS zone matches domain bindings %v", bindingIDs)
	}
	primary := zones[0]
	return ClusterDNSConfig{
		PrimaryZone:    primary,
		PortalHost:     PortalHost(primary),
		LLDAPWebUIHost: LLDAPWebUIHost(primary),
		AccessDomains:  AccessDomainsForZones(zones),
	}, nil
}

func matchedZones(site *fleetv1alpha1.FleetSite, bindingIDs []string) []string {
	byID := map[string]fleetv1alpha1.FleetSiteDomainSpec{}
	for _, d := range site.Spec.Edge.Domains {
		if d.ID != "" && d.Zone != "" {
			byID[d.ID] = d
		}
	}
	out := []string{}
	seen := map[string]struct{}{}
	for _, id := range bindingIDs {
		d, ok := byID[id]
		if !ok || d.Zone == "" {
			continue
		}
		if _, ok := seen[d.Zone]; ok {
			continue
		}
		seen[d.Zone] = struct{}{}
		out = append(out, d.Zone)
	}
	return out
}

func loadFleetSiteForJoin(ctx context.Context, c client.Client, ns string, fcj *fleetv1alpha1.FleetClusterJoin) (*fleetv1alpha1.FleetSite, error) {
	siteID := fcj.Spec.SiteId
	var site fleetv1alpha1.FleetSite
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: siteID}, &site); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("site %q not found", siteID)
		}
		return nil, err
	}
	return &site, nil
}

func loadEdgeClustersDNS(ctx context.Context, c client.Client, ns, name string) (edgeClustersDNSFile, error) {
	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return edgeClustersDNSFile{Clusters: map[string]edgeClusterDNS{}}, nil
		}
		return edgeClustersDNSFile{}, err
	}
	raw := cm.Data["clusters.yaml"]
	if raw == "" {
		return edgeClustersDNSFile{Clusters: map[string]edgeClusterDNS{}}, nil
	}
	var f edgeClustersDNSFile
	if err := yaml.Unmarshal([]byte(raw), &f); err != nil {
		return edgeClustersDNSFile{}, err
	}
	if f.Clusters == nil {
		f.Clusters = map[string]edgeClusterDNS{}
	}
	return f, nil
}

func DNSFromFCJStatus(fcj *fleetv1alpha1.FleetClusterJoin) ClusterDNSConfig {
	if fcj.Status.IngressAuth.PortalHost == "" {
		return ClusterDNSConfig{}
	}
	cookie := strings.TrimPrefix(fcj.Status.IngressAuth.PortalHost, "auth.")
	return ClusterDNSConfig{
		PrimaryZone:   cookie,
		PortalHost:    fcj.Status.IngressAuth.PortalHost,
		AccessDomains: append([]string(nil), fcj.Status.IngressAuth.AccessDomains...),
	}
}
