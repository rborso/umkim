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

type edgeClusterBinding struct {
	Site      string `yaml:"site"`
	IngressID string `yaml:"ingress_id,omitempty"`
}

type edgeClustersYAML struct {
	Clusters map[string]edgeClusterBinding `yaml:"clusters"`
}

// SyncSiteIngressAssignments derives FleetSite ingress assignedClusters from cluster ingress_id bindings.
func SyncSiteIngressAssignments(ctx context.Context, c client.Client, cfg DomainConfig) error {
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
		fs.Spec.NormalizeIngress()
		siteID := fs.Spec.EffectiveSiteID(fs.Name)
		mgmt := isManagementSite(fs, siteID, cfg.ManagementSiteID)
		changed := false
		for j := range fs.Spec.Edge.Ingress {
			ent := &fs.Spec.Edge.Ingress[j]
			var assigned []string
			if !mgmt {
				assigned = clustersForIngress(siteID, ent.ID, bindings, clusterSite)
			}
			if !stringSliceEqual(ent.AssignedClusters, assigned) {
				ent.AssignedClusters = assigned
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

// SyncSiteAssignedClusters is an alias for ingress assignment sync.
func SyncSiteAssignedClusters(ctx context.Context, c client.Client, cfg DomainConfig) error {
	return SyncSiteIngressAssignments(ctx, c, cfg)
}

func clustersForIngress(siteID, ingressID string, bindings map[string]edgeClusterBinding, clusterSite map[string]string) []string {
	out := []string{}
	for clusterName, ent := range bindings {
		if clusterSite[clusterName] != siteID {
			continue
		}
		if ent.IngressID == ingressID {
			out = append(out, clusterName)
		}
	}
	sort.Strings(out)
	return out
}

func loadEdgeClusterBindings(ctx context.Context, c client.Client, ns, name string) (map[string]edgeClusterBinding, error) {
	if ns == "" || name == "" {
		return map[string]edgeClusterBinding{}, nil
	}
	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return map[string]edgeClusterBinding{}, nil
		}
		return nil, err
	}
	raw := cm.Data["clusters.yaml"]
	if raw == "" {
		return map[string]edgeClusterBinding{}, nil
	}
	var f edgeClustersYAML
	if err := yaml.Unmarshal([]byte(raw), &f); err != nil {
		return nil, err
	}
	if f.Clusters == nil {
		return map[string]edgeClusterBinding{}, nil
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

func isManagementSite(fs *fleetv1alpha1.FleetSite, siteID, managementSiteID string) bool {
	if siteID == managementSiteID && managementSiteID != "" {
		return true
	}
	if fs.Annotations != nil && fs.Annotations[AnnotationManagementSite] == "true" {
		return true
	}
	return false
}
