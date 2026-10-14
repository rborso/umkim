package teardown

import (
	"context"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/registry"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// spokeKubeconfigSecretNS holds the hub-side kubeconfig Secret for each spoke.
const spokeKubeconfigSecretNS = "fleet-system"

// ManifestWorkNames matches inventory fleet_cleanup_manifestworks (hub-delivered spoke workloads).
var ManifestWorkNames = []string{
	"fleet-ingress-auth",
	"phase6-spoke-demo",
	"phase6-spoke-ingress",
	"phase5-helm-release-sp",
	"phase5-olm-subscription-sp",
	"phase5-k8s-container-sp",
	"phase4-platform-bootstrap",
}

var manifestWorkGVK = schema.GroupVersionKind{
	Group: "work.open-cluster-management.io", Version: "v1", Kind: "ManifestWork",
}

var managedClusterGVK = schema.GroupVersionKind{
	Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
}

// DeleteManifestWorks removes OCM ManifestWorks in namespace clusterName on the hub.
func DeleteManifestWorks(ctx context.Context, c client.Client, clusterName string) error {
	for _, name := range ManifestWorkNames {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(manifestWorkGVK)
		obj.SetName(name)
		obj.SetNamespace(clusterName)
		if err := c.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// DeleteManagedCluster removes the hub ManagedCluster (OCM unjoin).
func DeleteManagedCluster(ctx context.Context, c client.Client, clusterName string) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(managedClusterGVK)
	obj.SetName(clusterName)
	if err := c.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// ManagedClusterExists returns true when the hub has a ManagedCluster with the given name.
func ManagedClusterExists(ctx context.Context, c client.Client, clusterName string) bool {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(managedClusterGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: clusterName}, obj); err != nil {
		return false
	}
	return true
}

// ForceDeleteManagedCluster removes the hub ManagedCluster, clearing finalizers so
// deletion completes even when the spoke/klusterlet is unreachable (for example
// after the node was reinstalled). Returns nil when already gone.
func ForceDeleteManagedCluster(ctx context.Context, c client.Client, clusterName string) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(managedClusterGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: clusterName}, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if len(obj.GetFinalizers()) > 0 {
		obj.SetFinalizers(nil)
		if err := c.Update(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	if err := c.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

type edgeEntry struct {
	Site            string `yaml:"site"`
	DirectIngress   bool   `yaml:"direct_ingress"`
	IngressID       string `yaml:"ingress_id,omitempty"`
	Method          string `yaml:"method,omitempty"`
	TlsPolicy       string `yaml:"tls_policy,omitempty"`
	UpstreamHost    string `yaml:"upstream_host"`
	TraefikHTTPPort int    `yaml:"traefik_http_port"`
}

type edgeFile struct {
	Clusters map[string]edgeEntry `yaml:"clusters"`
}

// RemoveEdgeClusterEntry deletes clusterName from fleet-edge-clusters ConfigMap.
func RemoveEdgeClusterEntry(ctx context.Context, c client.Client, cfg config.Config, clusterName string) error {
	ns := cfg.EdgeClustersCMNamespace
	name := cfg.EdgeClustersCMName
	if ns == "" || name == "" {
		return nil
	}
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
	var f edgeFile
	if err := yaml.Unmarshal([]byte(raw), &f); err != nil {
		return err
	}
	if f.Clusters == nil {
		return nil
	}
	if _, ok := f.Clusters[clusterName]; !ok {
		return nil
	}
	delete(f.Clusters, clusterName)
	out, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	_, err = controllerutil.CreateOrUpdate(ctx, c, &cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data["clusters.yaml"] = string(out)
		if cm.Annotations == nil {
			cm.Annotations = map[string]string{}
		}
		cm.Annotations[registry.AnnotationConsoleManaged] = "true"
		return nil
	})
	return err
}

// ReconcileDomains runs site assignment sync and edge domain reconciliation after cluster removal.
func ReconcileDomains(ctx context.Context, c client.Client, cfg config.Config) error {
	domainCfg := registry.DomainConfig{
		FleetSiteNamespace:      cfg.Namespace,
		EdgeDomainsCMNamespace:  cfg.EdgeDomainsCMNamespace,
		EdgeDomainsCMName:       cfg.EdgeDomainsCMName,
		EdgeClustersCMNamespace: cfg.EdgeClustersCMNamespace,
		EdgeClustersCMName:      cfg.EdgeClustersCMName,
		HubEdgeDomain:           cfg.HubEdgeDomain,
		ManagementSiteID:        cfg.ManagementSiteID,
	}
	_ = registry.SyncSiteAssignedClusters(ctx, c, domainCfg)
	return registry.ReconcileEdgeDomains(ctx, c, domainCfg)
}

// clusterSecretNames are the per-cluster hub Secrets removed with the cluster.// The ingress-auth secret is included because the ingress-auth reconciler only
// builds desired state for Ready joins, so once the cluster is Deleting nothing
// recreates it — it would otherwise sit on the hub forever.
func clusterSecretNames(clusterName string) []string {
	return []string{
		"fleet-cluster-" + clusterName + "-bootstrap",
		"fleet-cluster-" + clusterName + "-ansible-ssh",
		"fleet-ingress-auth-" + clusterName,
	}
}

func clusterInventoryCM(clusterName string) string {
	return "fleet-cluster-" + clusterName + "-inventory"
}

// deleteClusterKubeconfigSecret removes the spoke kubeconfig the hub keeps for a
// cluster. It lives in the spoke-kubeconfig namespace, not the controller one.
func deleteClusterKubeconfigSecret(ctx context.Context, c client.Client, clusterName string) error {
	obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      "fleet-kubeconfig-" + clusterName,
		Namespace: spokeKubeconfigSecretNS,
	}}
	if err := c.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// DeleteClusterJoinArtifacts removes bootstrap secrets, inventory CM, and cluster-scoped jobs after FCJ delete.
func DeleteClusterJoinArtifacts(ctx context.Context, c client.Client, cfg config.Config, clusterName string) error {
	ns := cfg.Namespace
	for _, name := range clusterSecretNames(clusterName) {
		obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		if err := c.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	if err := deleteClusterKubeconfigSecret(ctx, c, clusterName); err != nil {
		return err
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: clusterInventoryCM(clusterName), Namespace: ns}}
	if err := c.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	var jobs batchv1.JobList
	if err := c.List(ctx, &jobs, client.InNamespace(ns)); err != nil {
		return err
	}
	prefix := "fleet-cluster-"
	for i := range jobs.Items {
		jn := jobs.Items[i].Name
		if strings.Contains(jn, clusterName) && strings.HasPrefix(jn, prefix) {
			if err := c.Delete(ctx, &jobs.Items[i], client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

// TeardownOrphanCluster removes leftover hub objects when FleetClusterJoin and ManagedCluster are already gone.
func TeardownOrphanCluster(ctx context.Context, c client.Client, cfg config.Config, clusterName string) error {
	ns := cfg.Namespace
	for _, name := range clusterSecretNames(clusterName) {
		obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		if err := c.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: clusterInventoryCM(clusterName), Namespace: ns}}
	if err := c.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err := deleteClusterKubeconfigSecret(ctx, c, clusterName); err != nil {
		return err
	}
	var jobs batchv1.JobList
	if err := c.List(ctx, &jobs, client.InNamespace(ns)); err != nil {
		return err
	}
	prefix := "fleet-cluster-"
	for i := range jobs.Items {
		jn := jobs.Items[i].Name
		if strings.Contains(jn, clusterName) && strings.HasPrefix(jn, prefix) {
			if err := c.Delete(ctx, &jobs.Items[i], client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	if err := RemoveEdgeClusterEntry(ctx, c, cfg, clusterName); err != nil {
		return err
	}
	return ReconcileDomains(ctx, c, cfg)
}

// TeardownInventoryCluster deletes MWs, MC, and edge registry entry for inventory OCM clusters (no FCJ).
func TeardownInventoryCluster(ctx context.Context, c client.Client, cfg config.Config, clusterName string) error {
	if err := DeleteManifestWorks(ctx, c, clusterName); err != nil {
		return err
	}
	if err := DeleteManagedCluster(ctx, c, clusterName); err != nil {
		return err
	}
	if err := RemoveEdgeClusterEntry(ctx, c, cfg, clusterName); err != nil {
		return err
	}
	return ReconcileDomains(ctx, c, cfg)
}

// HasCleanupFinalizer reports whether the FCJ has the cluster cleanup finalizer.
func HasCleanupFinalizer(obj metav1.Object) bool {
	for _, f := range obj.GetFinalizers() {
		if f == fleetv1alpha1.ClusterCleanupFinalizer {
			return true
		}
	}
	return false
}
