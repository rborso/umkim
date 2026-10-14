package httpapi

import (
	"context"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/teardown"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	annotationCleanupKubernetes = "fleet.umkim.com/cleanup-kubernetes"
	annotationCleanupVms        = "fleet.umkim.com/cleanup-vms"
	annotationCleanupVmDecls    = "fleet.umkim.com/cleanup-vm-declarations"
	annotationCleanupApps       = "fleet.umkim.com/cleanup-apps"
)

// clusterDeleteOptions captures the opt-in cleanup levels for a cluster delete.
// Destroying the VMs that run the cluster implies the Kubernetes reset, and
// removing the guest declarations implies destroying the VMs — each level needs
// the one below it. Uninstalling the applications is independent: it is a
// graceful spoke-side removal that is useful with or without a node reset.
type clusterDeleteOptions struct {
	cleanupKubernetes     bool
	cleanupVms            bool
	cleanupVmDeclarations bool
	cleanupApps           bool
}

func parseClusterDeleteOptions(r *http.Request) clusterDeleteOptions {
	opts := clusterDeleteOptions{
		cleanupKubernetes:     queryParamBool(r, "cleanupKubernetes"),
		cleanupVms:            queryParamBool(r, "cleanupVms"),
		cleanupVmDeclarations: queryParamBool(r, "cleanupVmDeclarations"),
		cleanupApps:           queryParamBool(r, "cleanupApps"),
	}
	if opts.cleanupVmDeclarations {
		opts.cleanupVms = true
	}
	if opts.cleanupVms {
		opts.cleanupKubernetes = true
	}
	return opts
}

func queryParamBool(r *http.Request, key string) bool {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	return v == "1" || v == "true" || v == "TRUE"
}

func (s *Server) handleDeleteCluster(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	_, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if s.clusterReadOnly(name, edge) || name == s.Cfg.ManagementClusterName {
		http.Error(w, "cluster "+name+" cannot be deleted", http.StatusForbidden)
		return
	}

	mc, mcErr := getManagedCluster(ctx, s.Client, name)
	_ = mcErr

	fcj, fcjErr := s.findFleetClusterJoin(ctx, name)
	if fcjErr == nil {
		if fcj.Status.Phase == fleetv1alpha1.JoinPhaseDeleting {
			writeJSON(w, http.StatusAccepted, map[string]string{
				"name":               name,
				"provisioning_phase": fleetv1alpha1.JoinPhaseDeleting,
			})
			return
		}
		if fcj.Status.Phase == fleetv1alpha1.JoinPhaseProvisioning {
			mc, mcErr := getManagedCluster(ctx, s.Client, name)
			if mcErr != nil || !mcConditionAvailable(mc) {
				if !queryParamBool(r, "force") && provisionJobActive(ctx, s.Client, fcj.Namespace, fcj.Status.ProvisionJobName) {
					http.Error(w, "cluster is provisioning; wait for completion or failure (or use ?force=true)", http.StatusConflict)
					return
				}
			}
		}
		if err := s.markClusterDeleting(ctx, fcj, parseClusterDeleteOptions(r)); err != nil {
			if apierrors.IsConflict(err) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{
			"name":               name,
			"provisioning_phase": fleetv1alpha1.JoinPhaseDeleting,
		})
		return
	}
	if !apierrors.IsNotFound(fcjErr) {
		http.Error(w, fcjErr.Error(), http.StatusInternalServerError)
		return
	}

	mc, mcErr = getManagedCluster(ctx, s.Client, name)
	if mcErr != nil {
		if apierrors.IsNotFound(mcErr) {
			_ = teardown.TeardownOrphanCluster(ctx, s.Client, s.Cfg, name)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, mcErr.Error(), http.StatusInternalServerError)
		return
	}
	_ = mc
	if err := teardown.TeardownInventoryCluster(ctx, s.Client, s.Cfg, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) markClusterDeleting(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, opts clusterDeleteOptions) error {
	key := client.ObjectKeyFromObject(fcj)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetClusterJoin
		if err := s.Client.Get(ctx, key, &latest); err != nil {
			return err
		}
		if latest.Status.Phase == fleetv1alpha1.JoinPhaseDeleting {
			return nil
		}
		metaOrig := latest.DeepCopy()
		if !controllerutil.ContainsFinalizer(&latest, fleetv1alpha1.ClusterCleanupFinalizer) {
			controllerutil.AddFinalizer(&latest, fleetv1alpha1.ClusterCleanupFinalizer)
		}
		if opts.cleanupKubernetes || opts.cleanupVms || opts.cleanupVmDeclarations || opts.cleanupApps {
			if latest.Annotations == nil {
				latest.Annotations = map[string]string{}
			}
		}
		if opts.cleanupKubernetes {
			latest.Annotations[annotationCleanupKubernetes] = "true"
		}
		if opts.cleanupVms {
			latest.Annotations[annotationCleanupVms] = "true"
		}
		if opts.cleanupVmDeclarations {
			latest.Annotations[annotationCleanupVmDecls] = "true"
		}
		if opts.cleanupApps {
			latest.Annotations[annotationCleanupApps] = "true"
		}
		if err := s.Client.Patch(ctx, &latest, client.MergeFrom(metaOrig)); err != nil {
			return err
		}
		if err := s.Client.Get(ctx, key, &latest); err != nil {
			return err
		}
		statusOrig := latest.DeepCopy()
		latest.Status.Phase = fleetv1alpha1.JoinPhaseDeleting
		latest.Status.Message = "Cluster deletion started"
		if latest.Status.DeletionStartedAt == nil {
			now := metav1.Now()
			latest.Status.DeletionStartedAt = &now
		}
		return s.Client.Status().Patch(ctx, &latest, client.MergeFrom(statusOrig))
	})
}

func provisionJobActive(ctx context.Context, c client.Client, ns, name string) bool {
	if name == "" {
		return false
	}
	var job batchv1.Job
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &job); err != nil {
		return false
	}
	for _, cond := range job.Status.Conditions {
		if (cond.Type == batchv1.JobComplete || cond.Type == batchv1.JobFailed) && cond.Status == "True" {
			return false
		}
	}
	return true
}
