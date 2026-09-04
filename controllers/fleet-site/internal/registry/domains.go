package registry

import (
	"context"
	"encoding/json"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type DomainConfig struct {
	FleetSiteNamespace      string
	EdgeDomainsCMNamespace  string
	EdgeDomainsCMName       string
	EdgeClustersCMNamespace string
	EdgeClustersCMName      string
	HubEdgeDomain           string
	ManagementSiteID        string
}

type domainRegistry struct {
	HubZone         string               `json:"hub_zone"`
	HomelabZone     string               `json:"homelab_zone,omitempty"`
	HubWildcard     string               `json:"hub_wildcard"`
	HomelabWildcard string               `json:"homelab_wildcard,omitempty"`
	Sites           []domainRegistrySite `json:"sites"`
	ClusterPatterns []map[string]string  `json:"cluster_patterns,omitempty"`
}

type domainRegistryIngress struct {
	ID               string   `json:"id"`
	Method           string   `json:"method"`
	Address          string   `json:"address,omitempty"`
	Zone             string   `json:"zone,omitempty"`
	Wildcard         string   `json:"wildcard,omitempty"`
	Description      string   `json:"description,omitempty"`
	OperatorNotes    string   `json:"operator_notes,omitempty"`
	AssignedClusters []string `json:"assigned_clusters,omitempty"`
}

type domainRegistrySite struct {
	ID            string                  `json:"id"`
	DisplayName   string                  `json:"display_name"`
	Zone          string                  `json:"zone"`
	Wildcard      string                  `json:"wildcard"`
	HubGateway    bool                    `json:"hub_gateway,omitempty"`
	HubZone       string                  `json:"hub_zone,omitempty"`
	HubWildcard   string                  `json:"hub_wildcard,omitempty"`
	Ingress       []domainRegistryIngress `json:"ingress,omitempty"`
	Clusters      []string                `json:"clusters"`
	DnsTargetNote string                  `json:"dns_target_note,omitempty"`
}

func ReconcileEdgeDomains(ctx context.Context, c client.Client, cfg DomainConfig) error {
	ns := cfg.EdgeDomainsCMNamespace
	name := cfg.EdgeDomainsCMName
	if ns == "" || name == "" {
		return nil
	}

	if err := SyncSiteAssignedClusters(ctx, c, cfg); err != nil {
		return err
	}

	var list fleetv1alpha1.FleetSiteList
	fleetNS := cfg.FleetSiteNamespace
	if fleetNS == "" {
		fleetNS = "fleet-console"
	}
	if err := c.List(ctx, &list, client.InNamespace(fleetNS)); err != nil {
		return err
	}

	mcClusters, err := clustersBySiteID(ctx, c)
	if err != nil {
		return err
	}

	hubZone := cfg.HubEdgeDomain
	if hubZone == "" {
		hubZone = "mgmt.umkim.com"
	}
	hubWildcard := "*." + hubZone

	reg := domainRegistry{
		HubZone:     hubZone,
		HubWildcard: hubWildcard,
		Sites:       []domainRegistrySite{},
		ClusterPatterns: []map[string]string{
			{"pattern": "{app}." + hubZone, "description": "Hub-routed app hostname"},
			{"pattern": "{app}.{cluster}." + hubZone, "description": "Per-cluster site hostname"},
		},
	}

	for i := range list.Items {
		fs := &list.Items[i]
		fs.Spec.NormalizeIngress()
		siteID := fs.Spec.EffectiveSiteID(fs.Name)
		edgeDomain := fs.Spec.EffectiveEdgeDomain()
		wildcard := fs.Spec.EffectiveWildcard()
		display := fs.Spec.DisplayName
		if display == "" {
			display = siteID
		}
		clusters := mcClusters[siteID]
		if clusters == nil {
			clusters = []string{}
		}
		dnsNote := ""
		if we := fs.Spec.EffectiveWanEgress(); we != nil {
			dnsNote = we.DnsTargetNote
		}
		ingressRows := make([]domainRegistryIngress, 0, len(fs.Spec.Edge.Ingress))
		for _, ent := range fs.Spec.Edge.Ingress {
			wc := ent.EffectiveWildcard()
			assigned := ent.AssignedClusters
			if assigned == nil {
				assigned = []string{}
			}
			ingressRows = append(ingressRows, domainRegistryIngress{
				ID: ent.ID, Method: ent.Method, Address: ent.Address, Zone: ent.Zone,
				Wildcard: wc, Description: ent.Description, OperatorNotes: ent.OperatorNotes,
				AssignedClusters: assigned,
			})
		}
		reg.Sites = append(reg.Sites, domainRegistrySite{
			ID: siteID, DisplayName: display, Zone: edgeDomain, Wildcard: wildcard,
			HubGateway: fs.Spec.HubGatewayEnabled(), HubZone: fs.Spec.EffectiveHubZone(),
			HubWildcard: fs.Spec.EffectiveHubWildcard(), Ingress: ingressRows,
			Clusters: clusters, DnsTargetNote: dnsNote,
		})
		if siteID == "homelab" {
			reg.HomelabZone = edgeDomain
			reg.HomelabWildcard = wildcard
		}
		if cfg.ManagementSiteID != "" && siteID == cfg.ManagementSiteID {
			reg.HubZone = edgeDomain
			reg.HubWildcard = wildcard
			if reg.HubWildcard == "" && edgeDomain != "" {
				reg.HubWildcard = "*." + edgeDomain
			}
		}
	}

	raw, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "fleet-edge",
				"app.kubernetes.io/component": "domain-registry",
			},
			Annotations: map[string]string{
				AnnotationConsoleManaged: "true",
			},
		},
		Data: map[string]string{"domains.json": string(raw)},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, c, cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data["domains.json"] = string(raw)
		if cm.Annotations == nil {
			cm.Annotations = map[string]string{}
		}
		cm.Annotations[AnnotationConsoleManaged] = "true"
		return nil
	})
	if err != nil && apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// ClustersBySiteID lists OCM ManagedCluster names grouped by fleet.site label.
func ClustersBySiteID(ctx context.Context, c client.Client) (map[string][]string, error) {
	return clustersBySiteID(ctx, c)
}

func clustersBySiteID(ctx context.Context, c client.Client) (map[string][]string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedClusterList",
	})
	if err := c.List(ctx, list); err != nil {
		if apierrors.IsNotFound(err) {
			return map[string][]string{}, nil
		}
		return nil, err
	}
	out := map[string][]string{}
	for _, item := range list.Items {
		name := item.GetName()
		if name == "local-cluster" {
			continue
		}
		site := item.GetLabels()["fleet.site"]
		if site == "" {
			continue
		}
		out[site] = append(out[site], name)
	}
	return out, nil
}

// ConfigMapConsoleManaged reports whether Ansible should skip overwriting a CM.
func ConfigMapConsoleManaged(ctx context.Context, c client.Client, ns, name string) (bool, error) {
	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return cm.Annotations != nil && cm.Annotations[AnnotationConsoleManaged] == "true", nil
}
