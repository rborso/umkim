package ingressauth

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingress"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClusterDNSConfig struct {
	PrimaryZone    string
	PortalHost     string
	LLDAPWebUIHost string
	AccessDomains  []string
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
	Site      string `yaml:"site"`
	IngressID string `yaml:"ingress_id,omitempty"`
}

type edgeClustersDNSFile struct {
	Clusters map[string]edgeClusterDNS `yaml:"clusters"`
}

func ResolveClusterDNS(ctx context.Context, c client.Client, cfg config.Config, clusterName string, fcj *fleetv1alpha1.FleetClusterJoin) (ClusterDNSConfig, error) {
	ingressID := strings.TrimSpace(fcj.Spec.Edge.IngressId)
	if ingressID == "" {
		edge, err := loadEdgeClustersDNS(ctx, c, cfg.EdgeClustersCMNamespace, cfg.EdgeClustersCMName)
		if err != nil {
			return ClusterDNSConfig{}, err
		}
		if ent, ok := edge.Clusters[clusterName]; ok {
			ingressID = ent.IngressID
		}
	}
	if ingressID == "" {
		return ClusterDNSConfig{}, fmt.Errorf("cluster has no ingress binding; configure ingressId before enabling ingress auth")
	}

	site, err := loadFleetSiteForJoin(ctx, c, cfg.Namespace, fcj)
	if err != nil {
		return ClusterDNSConfig{}, err
	}

	ci, err := ingress.ResolveClusterIngress(site.Spec, ingressID)
	if err != nil {
		return ClusterDNSConfig{}, err
	}
	if !ci.AllowsAuth || ci.Zone == "" {
		return ClusterDNSConfig{}, fmt.Errorf("ingress auth requires port_forward ingress with a DNS zone")
	}
	return ClusterDNSConfig{
		PrimaryZone:    ci.Zone,
		PortalHost:     PortalHost(ci.Zone),
		LLDAPWebUIHost: LLDAPWebUIHost(ci.Zone),
		AccessDomains:  AccessDomainsForZones([]string{ci.Zone}),
	}, nil
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
