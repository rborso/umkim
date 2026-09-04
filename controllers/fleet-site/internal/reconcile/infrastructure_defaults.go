package reconcile

import (
	"context"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/acme"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingress"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingressauth"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type infraEdgeClusterEntry struct {
	DirectIngress bool   `yaml:"direct_ingress"`
	TlsPolicy     string `yaml:"tls_policy,omitempty"`
	Method        string `yaml:"method,omitempty"`
	IngressID     string `yaml:"ingress_id,omitempty"`
}

type infraEdgeClustersFile struct {
	Clusters map[string]infraEdgeClusterEntry `yaml:"clusters"`
}

func loadInfraEdgeClusters(ctx context.Context, c client.Client, ns, name string) (infraEdgeClustersFile, error) {
	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return infraEdgeClustersFile{Clusters: map[string]infraEdgeClusterEntry{}}, nil
		}
		return infraEdgeClustersFile{}, err
	}
	raw := cm.Data["clusters.yaml"]
	if raw == "" {
		return infraEdgeClustersFile{Clusters: map[string]infraEdgeClusterEntry{}}, nil
	}
	var f infraEdgeClustersFile
	if err := yaml.Unmarshal([]byte(raw), &f); err != nil {
		return infraEdgeClustersFile{}, err
	}
	if f.Clusters == nil {
		f.Clusters = map[string]infraEdgeClusterEntry{}
	}
	return f, nil
}

func (r *AcmeReconciler) maybeSyncInventoryAcme(ctx context.Context, fa *fleetv1alpha1.FleetAcme) (bool, error) {
	if !r.Cfg.FleetEdgeACMEEnabled {
		return false, nil
	}
	changed := false
	if !fa.Spec.HubEnabled {
		fa.Spec.HubEnabled = true
		changed = true
	}
	if fa.Spec.Email == "" && r.Cfg.FleetEdgeACMEEmail != "" {
		fa.Spec.Email = r.Cfg.FleetEdgeACMEEmail
		changed = true
	}
	if !changed {
		return false, nil
	}
	if fa.Annotations == nil {
		fa.Annotations = map[string]string{}
	}
	fa.Annotations[acme.ReconcileAnnotation] = "inventory-sync"
	if err := r.Update(ctx, fa); err != nil {
		return false, err
	}
	return true, nil
}

func (r *IngressAuthReconciler) maybeSyncInventoryIngressAuth(ctx context.Context, fia *fleetv1alpha1.FleetIngressAuth) (bool, error) {
	if !r.Cfg.FleetIngressAuthEnabled {
		return false, nil
	}
	secretRef := fia.Spec.EffectiveSecretsRef()
	if r.Cfg.FleetIngressAuthBootstrapPassword != "" {
		if err := ingressauth.EnsureHubSecretFromInventory(ctx, r.Client, r.Cfg.Namespace, secretRef, r.Cfg.FleetIngressAuthBootstrapPassword); err != nil {
			return false, err
		}
	} else if !ingressauth.HubSecretReady(ctx, r.Client, r.Cfg.Namespace, secretRef) {
		return false, nil
	}
	if fia.Spec.HubEnabled {
		return false, nil
	}
	fia.Spec.HubEnabled = true
	if fia.Annotations == nil {
		fia.Annotations = map[string]string{}
	}
	fia.Annotations[ingressauth.ReconcileAnnotation] = "inventory-sync"
	if err := r.Update(ctx, fia); err != nil {
		return false, err
	}
	return true, nil
}

func (r *ClusterJoinReconciler) applyInventoryInfrastructureDefaults(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) error {
	specChanged := false
	statusChanged := false

	if r.Cfg.FleetFCJDefaultAcmeEnabled && !fcj.Spec.Acme.Enabled && r.clusterAcmeEligible(ctx, clusterName, fcj) && r.clusterDirectIngress(ctx, clusterName, fcj) {
		fcj.Spec.Acme.Enabled = true
		fcj.Status.Acme = fleetv1alpha1.FleetClusterJoinAcmeStatus{
			Phase:   fleetv1alpha1.AcmeClusterPhasePending,
			Message: "Cluster ACME enabled from inventory defaults",
		}
		specChanged = true
		statusChanged = true
	}

	if r.Cfg.FleetFCJDefaultIngressAuthEnabled && !fcj.Spec.IngressAuth.Enabled && r.clusterIngressEnabled(ctx, clusterName, fcj) {
		password := strings.TrimSpace(r.Cfg.FleetIngressAuthBootstrapPassword)
		if len(password) >= ingressauth.MinLLDAPAdminPasswordLen {
			dns, err := ingressauth.ResolveClusterDNS(ctx, r.Client, r.Cfg, clusterName, fcj)
			if err == nil {
				secretRef, serr := ingressauth.EnsureClusterSecret(ctx, r.Client, r.Cfg.Namespace, clusterName, password)
				if serr == nil {
					fcj.Spec.IngressAuth.Enabled = true
					fcj.Status.IngressAuth = fleetv1alpha1.FleetClusterJoinIngressAuthStatus{
						Phase:         fleetv1alpha1.IngressAuthClusterPhasePending,
						Message:       "Ingress auth enabled from inventory defaults",
						SecretsRef:    secretRef,
						PortalHost:    dns.PortalHost,
						AccessDomains: dns.AccessDomains,
					}
					specChanged = true
					statusChanged = true
				}
			}
		}
	}

	if !specChanged && !statusChanged {
		return nil
	}
	if specChanged {
		if err := r.Update(ctx, fcj); err != nil {
			return err
		}
	}
	if statusChanged {
		if err := r.Status().Update(ctx, fcj); err != nil {
			return err
		}
	}
	if fcj.Spec.Acme.Enabled {
		if err := r.triggerAcmeReconcile(ctx); err != nil {
			return err
		}
	}
	if fcj.Spec.IngressAuth.Enabled {
		return r.triggerIngressAuthReconcile(ctx)
	}
	return nil
}

func (r *ClusterJoinReconciler) triggerAcmeReconcile(ctx context.Context) error {
	fa, err := acme.EnsureCR(ctx, r.Client, r.Cfg.Namespace, r.Cfg)
	if err != nil {
		return err
	}
	if fa.Annotations == nil {
		fa.Annotations = map[string]string{}
	}
	fa.Annotations[acme.ReconcileAnnotation] = time.Now().UTC().Format(time.RFC3339)
	return r.Update(ctx, fa)
}

func (r *ClusterJoinReconciler) clusterAcmeEligible(ctx context.Context, clusterName string, fcj *fleetv1alpha1.FleetClusterJoin) bool {
	ingressID := strings.TrimSpace(fcj.Spec.Edge.IngressId)
	edge, err := loadInfraEdgeClusters(ctx, r.Client, r.Cfg.EdgeClustersCMNamespace, r.Cfg.EdgeClustersCMName)
	if err == nil {
		if e, ok := edge.Clusters[clusterName]; ok {
			if e.Method == fleetv1alpha1.IngressMethodReverseProxy {
				return false
			}
			if e.IngressID != "" {
				ingressID = e.IngressID
			}
			if e.TlsPolicy != "" {
				return e.TlsPolicy == fleetv1alpha1.TLSPolicyACMECluster
			}
		}
	}
	if ingressID == "" {
		return false
	}
	var fs fleetv1alpha1.FleetSite
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: fcj.Spec.SiteId}, &fs); err != nil {
		return false
	}
	ci, err := ingress.ResolveClusterIngress(fs.Spec, ingressID)
	return err == nil && ci.AllowsSpokeACME
}

func (r *ClusterJoinReconciler) clusterDirectIngress(ctx context.Context, clusterName string, fcj *fleetv1alpha1.FleetClusterJoin) bool {
	edge, err := loadInfraEdgeClusters(ctx, r.Client, r.Cfg.EdgeClustersCMNamespace, r.Cfg.EdgeClustersCMName)
	if err != nil {
		return false
	}
	if e, ok := edge.Clusters[clusterName]; ok {
		return e.DirectIngress
	}
	return r.clusterIngressEnabled(ctx, clusterName, fcj)
}

func (r *ClusterJoinReconciler) clusterIngressEnabled(ctx context.Context, clusterName string, fcj *fleetv1alpha1.FleetClusterJoin) bool {
	ingressID := strings.TrimSpace(fcj.Spec.Edge.IngressId)
	edge, err := loadInfraEdgeClusters(ctx, r.Client, r.Cfg.EdgeClustersCMNamespace, r.Cfg.EdgeClustersCMName)
	if err == nil {
		if e, ok := edge.Clusters[clusterName]; ok && e.IngressID != "" {
			ingressID = e.IngressID
		}
	}
	if ingressID == "" {
		return false
	}
	var fs fleetv1alpha1.FleetSite
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: fcj.Spec.SiteId}, &fs); err != nil {
		return false
	}
	ci, err := ingress.ResolveClusterIngress(fs.Spec, ingressID)
	return err == nil && ci.AllowsAuth
}
