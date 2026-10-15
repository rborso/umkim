package httpapi

import (
	"context"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ansiblelogs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/provision"
	"github.com/rborso/umkim/controllers/fleet-site/internal/siteinventory"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// refreshInventoryJobs finalizes recent inventory sync/import batch jobs.
func (s *Server) refreshInventoryJobs(ctx context.Context, fs *fleetv1alpha1.FleetSite) {
	cs, err := s.localClientset()
	if err != nil || cs == nil {
		return
	}
	var list batchv1.JobList
	if err := s.Client.List(ctx, &list, client.InNamespace(s.Cfg.Namespace), client.MatchingLabels{
		"fleet-site": fs.Name,
	}); err != nil {
		return
	}
	for i := range list.Items {
		job := &list.Items[i]
		if !strings.HasPrefix(job.Name, siteinventory.JobPrefix) {
			continue
		}
		if job.Status.Succeeded == 0 && job.Status.Failed == 0 {
			continue
		}
		logs, _ := ansiblelogs.PodLogs(ctx, cs, s.Cfg.Namespace, job.Name)
		if job.Status.Succeeded > 0 {
			if remoteFiles, ok := provision.ParseInventoryFiles(logs); ok {
				localData, _ := connectors.Load(ctx, s.Client, s.Cfg.Namespace, fs.Name, fleetv1alpha1.ProvisionProviderYaidap)
				changes := yaidap.CompareImportFiles(localData, remoteFiles)
				_ = s.storeImportPreview(ctx, fs.Name, changes)
			}
			importFailed := false
			if pulled, ok := provision.ParseInventoryImport(logs); ok {
				if err := yaidap.ApplyImport(ctx, s.Client, s.Cfg.Namespace, fs.Name, "system/import", pulled, true); err != nil {
					importFailed = true
					_ = yaidap.MarkImportFailed(ctx, s.Client, s.Cfg.Namespace, fs.Name, err)
				}
			}
			if !importFailed && provision.InventoryJobSucceeded(logs) {
				path := ""
				if fs.Spec.Provision != nil {
					path = fs.Spec.Provision.InventoryPath
				}
				_ = yaidap.FinalizeSyncFromJobLogs(ctx, s.Client, s.Cfg.Namespace, fs.Name, path, logs)
			}
		}
		// Foreground: the Job's pods go with it. Background deletion can leave them
		// orphaned, and the pod GC only reclaims orphaned terminated pods past a
		// high water mark, so they would accumulate on the hub.
		_ = s.Client.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground))
	}
}
