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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	annotationCleanupKubernetes = "fleet.umkim.com/cleanup-kubernetes"
)

func queryParamBool(r *http.Request, key string) bool {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	return v == "1" || v == "true" || v == "TRUE"
}

func (s *Server) handleDeleteCluster(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	actor, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if s.clusterReadOnly(name, edge) || name == s.Cfg.ManagementClusterName {
		http.Error(w, "cluster "+name+" cannot be deleted", http.StatusForbidden)
		return
	}

	mc, mcErr := getManagedCluster(ctx, s.Client, name)
	if mcErr == nil {
		if err := validateClusterDelete(actor, mcLabels(mc)); err != nil {
			if writeTenancyError(w, err) {
				return
			}
		}
	}

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
		cleanupKubernetes := queryParamBool(r, "cleanupKubernetes")
		original := fcj.DeepCopy()
		if !controllerutil.ContainsFinalizer(fcj, fleetv1alpha1.ClusterCleanupFinalizer) {
			controllerutil.AddFinalizer(fcj, fleetv1alpha1.ClusterCleanupFinalizer)
		}
		if cleanupKubernetes {
			if fcj.Annotations == nil {
				fcj.Annotations = map[string]string{}
			}
			fcj.Annotations[annotationCleanupKubernetes] = "true"
		}
		fcj.Status.Phase = fleetv1alpha1.JoinPhaseDeleting
		fcj.Status.Message = "Cluster deletion started"
		if fcj.Status.DeletionStartedAt == nil {
			now := metav1.Now()
			fcj.Status.DeletionStartedAt = &now
		}
		if err := s.Client.Patch(ctx, fcj, client.MergeFrom(original)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.Client.Status().Patch(ctx, fcj, client.MergeFrom(original)); err != nil {
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
