package registry

import (
	"context"
	"reflect"
	"sort"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type edgeClusterBindings struct {
	Site           string   `yaml:"site"`
	DomainBindings []string `yaml:"domain_bindings,omitempty"`
}

type edgeClustersYAML struct {
	Clusters map[string]edgeClusterBindings `yaml:"clusters"`
}

// SyncSiteAssignedClusters derives FleetSite domain assignedClusters from cluster domain_bindings.
func SyncSiteAssignedClusters(ctx context.Context, c client.Client, cfg DomainConfig) error {
	bindings, err := loadEdgeClusterBindings(ctx, c, cfg.EdgeClustersCMNamespace, cfg.EdgeClustersCMName)
	if err != nil {
		return err
	}
	mcSite, err := clustersBySiteID(ctx, c)
	if err != nil {
		return err
	}
	clusterSite := map[string]string{}
	for site, names := range mcSite {
		for _, n := range names {
			clusterSite[n] = site
		}
	}
	for name, ent := range bindings {
		if ent.Site != "" {
			clusterSite[name] = ent.Site
		}
	}

	var list fleetv1alpha1.FleetSiteList
	fleetNS := cfg.FleetSiteNamespace
	if fleetNS == "" {
		fleetNS = "fleet-console"
	}
	if err := c.List(ctx, &list, client.InNamespace(fleetNS)); err != nil {
		return err
	}

	for i := range list.Items {
		fs := &list.Items[i]
		fs.Spec.NormalizeEdge()
		siteID := fs.Spec.EffectiveSiteID(fs.Name)
		mgmt := isManagementSite(fs, siteID, cfg.ManagementSiteID)
		changed := false
		for j := range fs.Spec.Edge.Domains {
			d := &fs.Spec.Edge.Domains[j]
			var assigned []string
			if !mgmt {
				assigned = clustersForDomain(siteID, d.ID, bindings, clusterSite)
			}
			if !stringSliceEqual(d.AssignedClusters, assigned) {
				d.AssignedClusters = assigned
				changed = true
			}
		}
		if changed {
			if err := c.Update(ctx, fs); err != nil {
				return err
			}
		}
	}
	return nil
}

func isManagementSite(fs *fleetv1alpha1.FleetSite, siteID, managementSiteID string) bool {
	if managementSiteID != "" && siteID == managementSiteID {
		return true
	}
	return fs.Annotations != nil && fs.Annotations[AnnotationManagementSite] == "true"
}

func clustersForDomain(siteID, domainID string, bindings map[string]edgeClusterBindings, clusterSite map[string]string) []string {
	out := []string{}
	for clusterName, ent := range bindings {
		if clusterSite[clusterName] != siteID {
			continue
		}
		for _, b := range ent.DomainBindings {
			if b == domainID {
				out = append(out, clusterName)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func loadEdgeClusterBindings(ctx context.Context, c client.Client, ns, name string) (map[string]edgeClusterBindings, error) {
	if ns == "" || name == "" {
		return map[string]edgeClusterBindings{}, nil
	}
	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return map[string]edgeClusterBindings{}, nil
		}
		return nil, err
	}
	raw := cm.Data["clusters.yaml"]
	if raw == "" {
		return map[string]edgeClusterBindings{}, nil
	}
	var f edgeClustersYAML
	if err := yaml.Unmarshal([]byte(raw), &f); err != nil {
		return nil, err
	}
	if f.Clusters == nil {
		return map[string]edgeClusterBindings{}, nil
	}
	return f.Clusters, nil
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	return reflect.DeepEqual(ac, bc)
}
