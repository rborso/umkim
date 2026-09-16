package registry

import (
	"context"
	"reflect"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingress"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type edgeClusterSyncEntry struct {
	Site            string `yaml:"site"`
	Management      bool   `yaml:"management,omitempty"`
	ReadOnly        bool   `yaml:"read_only,omitempty"`
	DirectIngress   bool   `yaml:"direct_ingress"`
	IngressID       string `yaml:"ingress_id,omitempty"`
	Method          string `yaml:"method,omitempty"`
	TlsPolicy       string `yaml:"tls_policy,omitempty"`
	DNSZone         string `yaml:"dns_zone,omitempty"`
	EdgeWANIP       string `yaml:"edge_wan_ip,omitempty"`
	WanAddressMode  string `yaml:"wan_address_mode,omitempty"`
	UpstreamHost    string `yaml:"upstream_host"`
	TraefikHTTPPort int    `yaml:"traefik_http_port"`
}

type edgeClustersSyncFile struct {
	Clusters map[string]edgeClusterSyncEntry `yaml:"clusters"`
}

// SyncEdgeClustersFromSiteIngress refreshes fleet-edge-clusters ingress fields for clusters bound to the site.
func SyncEdgeClustersFromSiteIngress(ctx context.Context, c client.Client, cfg DomainConfig, fs *fleetv1alpha1.FleetSite) error {
	if fs == nil {
		return nil
	}
	ns := cfg.EdgeClustersCMNamespace
	name := cfg.EdgeClustersCMName
	if ns == "" || name == "" {
		return nil
	}
	siteID := fs.Spec.EffectiveSiteID(fs.Name)
	fs.Spec.NormalizeIngress()

	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	raw := cm.Data["clusters.yaml"]
	if raw == "" {
		return nil
	}
	var f edgeClustersSyncFile
	if err := yaml.Unmarshal([]byte(raw), &f); err != nil {
		return err
	}
	if f.Clusters == nil {
		return nil
	}

	changed := false
	for clusterName, ent := range f.Clusters {
		if ent.Site != siteID || strings.TrimSpace(ent.IngressID) == "" {
			continue
		}
		resolved, err := ingress.ResolveEdgeClusterIngress(fs.Spec, ent.IngressID)
		if err != nil {
			continue
		}
		next := ent
		next.Method = resolved.Method
		next.TlsPolicy = resolved.TlsPolicy
		next.DNSZone = resolved.DNSZone
		next.EdgeWANIP = resolved.EdgeWANIP
		next.WanAddressMode = resolved.WanAddressMode
		next.DirectIngress = resolved.DirectIngress
		if !reflect.DeepEqual(next, ent) {
			f.Clusters[clusterName] = next
			changed = true
		}
	}
	if !changed {
		return nil
	}

	out, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	patchCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, c, patchCM, func() error {
		if patchCM.Data == nil {
			patchCM.Data = map[string]string{}
		}
		patchCM.Data["clusters.yaml"] = string(out)
		if patchCM.Annotations == nil {
			patchCM.Annotations = map[string]string{}
		}
		patchCM.Annotations[AnnotationConsoleManaged] = "true"
		return nil
	})
	return err
}
