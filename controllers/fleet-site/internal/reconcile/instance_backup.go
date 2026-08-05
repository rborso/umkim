package reconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/backup"
	"github.com/rborso/umkim/controllers/fleet-site/internal/dcm"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/workload"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type InstanceBackupReconciler struct {
	*Reconciler
}

func (r *InstanceBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var fib fleetv1alpha1.FleetInstanceBackup
	if err := r.Get(ctx, req.NamespacedName, &fib); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if fib.Status.Phase == "" {
		fib.Status.Phase = fleetv1alpha1.BackupPhasePending
	}
	switch fib.Status.Phase {
	case fleetv1alpha1.BackupPhaseComplete, fleetv1alpha1.BackupPhaseFailed:
		return ctrl.Result{}, nil
	case fleetv1alpha1.BackupPhasePending:
		if fib.Status.StartedAt == nil {
			fib.Status.StartedAt = backup.NowMeta()
		}
		fib.Status.Phase = fleetv1alpha1.BackupPhaseRunning
		fib.Status.Message = "Backup job starting"
		if err := r.ensureBackupInventoryCM(ctx); err != nil {
			return r.failBackup(ctx, &fib, err)
		}
		jobName := backup.BackupJobName(fib.Name)
		veleroName := fib.Status.VeleroBackupName
		if veleroName == "" {
			veleroName = fmt.Sprintf("instance-%s-%s", fib.Spec.Release, fib.CreationTimestamp.UTC().Format("20060102150405"))
			fib.Status.VeleroBackupName = veleroName
		}
		secretName := backup.BackupSecretName(fib.Name)
		fib.Status.ArtifactSecret = secretName
		fib.Status.JobName = jobName
		if err := r.Status().Update(ctx, &fib); err != nil {
			return ctrl.Result{}, err
		}
		extra := []string{
			"-e", "fleet_instance_backup_cluster=" + fib.Spec.Cluster,
			"-e", "fleet_instance_backup_instance_id=" + fib.Spec.InstanceID,
			"-e", "fleet_instance_backup_release=" + fib.Spec.Release,
			"-e", "fleet_instance_backup_namespace=" + defaultNS(fib.Spec.Namespace),
			"-e", fmt.Sprintf("fleet_instance_backup_wait=%t", fib.Spec.Wait),
			"-e", "fleet_instance_backup_cr_name=" + fib.Name,
			"-e", "fleet_instance_backup_velero_name=" + veleroName,
			"-e", "fleet_instance_backup_artifact_secret=" + secretName,
			"-e", "fleet_site_dcm_api_url=" + r.Cfg.DCMCatalogAPIURL,
		}
		job := jobs.InstanceBackupJob(r.Cfg.Namespace, jobName, r.Cfg.AnsibleRunnerImage, jobs.InstanceBackupInventoryCM, extra)
		if err := controllerutil.SetControllerReference(&fib, job, r.Scheme); err != nil {
			return r.failBackup(ctx, &fib, err)
		}
		if err := r.ensureProviderJob(ctx, job); err != nil {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	case fleetv1alpha1.BackupPhaseRunning:
		if fib.Status.JobName == "" {
			return r.failBackup(ctx, &fib, fmt.Errorf("missing job name"))
		}
		done, ok, msg := r.jobDone(ctx, fib.Namespace, fib.Status.JobName)
		if !done {
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		if !ok {
			return r.failBackup(ctx, &fib, fmt.Errorf("%s", msg))
		}
		fib.Status.Phase = fleetv1alpha1.BackupPhaseComplete
		fib.Status.Message = "Backup complete"
		fib.Status.CompletedAt = backup.NowMeta()
		fib.Status.ObservedGeneration = fib.Generation
		if err := r.Status().Update(ctx, &fib); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("instance backup complete", "name", fib.Name, "velero", fib.Status.VeleroBackupName)
		return ctrl.Result{}, nil
	default:
		return ctrl.Result{}, nil
	}
}

func (r *InstanceBackupReconciler) failBackup(ctx context.Context, fib *fleetv1alpha1.FleetInstanceBackup, err error) (ctrl.Result, error) {
	fib.Status.Phase = fleetv1alpha1.BackupPhaseFailed
	fib.Status.Message = err.Error()
	fib.Status.CompletedAt = backup.NowMeta()
	_ = r.Status().Update(ctx, fib)
	return ctrl.Result{}, nil
}

func (r *InstanceBackupReconciler) ensureBackupInventoryCM(ctx context.Context) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobs.InstanceBackupInventoryCM,
			Namespace: r.Cfg.Namespace,
		},
		Data: map[string]string{
			"inventory.ini": "[hub]\nlocalhost ansible_connection=local\n",
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data["inventory.ini"] = "[hub]\nlocalhost ansible_connection=local\n"
		return nil
	})
	return err
}

type InstanceRestoreReconciler struct {
	*Reconciler
}

func (r *InstanceRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var fir fleetv1alpha1.FleetInstanceRestore
	if err := r.Get(ctx, req.NamespacedName, &fir); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if fir.Status.Phase == "" {
		fir.Status.Phase = fleetv1alpha1.RestorePhasePending
	}
	switch fir.Status.Phase {
	case fleetv1alpha1.RestorePhaseComplete, fleetv1alpha1.RestorePhaseFailed:
		return ctrl.Result{}, nil
	case fleetv1alpha1.RestorePhasePending:
		if err := r.resolveRestoreSpec(ctx, &fir); err != nil {
			return r.failRestore(ctx, &fir, err)
		}
		if fir.Status.StartedAt == nil {
			fir.Status.StartedAt = backup.NowMeta()
		}
		fir.Status.Phase = fleetv1alpha1.RestorePhaseRestoring
		fir.Status.Message = "Velero restore starting"
		jobName := backup.RestoreJobName(fir.Name)
		restoreName := fmt.Sprintf("restore-%s-%d", fir.Spec.Release, time.Now().Unix()%100000)
		fir.Status.VeleroRestoreName = restoreName
		fir.Status.JobName = jobName
		if err := r.ensureBackupInventoryCM(ctx); err != nil {
			return r.failRestore(ctx, &fir, err)
		}
		if err := r.Status().Update(ctx, &fir); err != nil {
			return ctrl.Result{}, err
		}
		extra := []string{
			"-e", "fleet_instance_restore_target_cluster=" + fir.Spec.TargetCluster,
			"-e", "fleet_instance_restore_release=" + fir.Spec.Release,
			"-e", "fleet_instance_restore_namespace=" + defaultNS(fir.Spec.Namespace),
			"-e", "fleet_instance_restore_velero_backup=" + fir.Spec.VeleroBackupName,
			"-e", "fleet_instance_restore_velero_restore_name=" + restoreName,
		}
		job := jobs.InstanceRestoreJob(r.Cfg.Namespace, jobName, r.Cfg.AnsibleRunnerImage, jobs.InstanceBackupInventoryCM, extra)
		if err := controllerutil.SetControllerReference(&fir, job, r.Scheme); err != nil {
			return r.failRestore(ctx, &fir, err)
		}
		if err := r.ensureProviderJob(ctx, job); err != nil {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	case fleetv1alpha1.RestorePhaseRestoring:
		done, ok, msg := r.jobDone(ctx, fir.Namespace, fir.Status.JobName)
		if !done {
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		if !ok {
			return r.failRestore(ctx, &fir, fmt.Errorf("%s", msg))
		}
		if fir.Spec.Rebind != nil && fir.Spec.Rebind.Enabled {
			fir.Status.Phase = fleetv1alpha1.RestorePhaseRebinding
			fir.Status.Message = "Rebinding DCM instance"
			if err := r.Status().Update(ctx, &fir); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		return r.completeRestore(ctx, &fir, "")
	case fleetv1alpha1.RestorePhaseRebinding:
		newID, err := r.rebindDCMInstance(ctx, &fir)
		if err != nil {
			return r.failRestore(ctx, &fir, err)
		}
		return r.completeRestore(ctx, &fir, newID)
	default:
		return ctrl.Result{}, nil
	}
}

func (r *InstanceRestoreReconciler) resolveRestoreSpec(ctx context.Context, fir *fleetv1alpha1.FleetInstanceRestore) error {
	if fir.Spec.VeleroBackupName != "" && fir.Spec.Release != "" {
		return nil
	}
	if fir.Spec.BackupCRName != "" {
		var fib fleetv1alpha1.FleetInstanceBackup
		if err := r.Get(ctx, client.ObjectKey{Namespace: fir.Namespace, Name: fir.Spec.BackupCRName}, &fib); err != nil {
			return err
		}
		if fir.Spec.VeleroBackupName == "" {
			fir.Spec.VeleroBackupName = fib.Status.VeleroBackupName
		}
		if fir.Spec.InstanceID == "" {
			fir.Spec.InstanceID = fib.Spec.InstanceID
		}
		if fir.Spec.Release == "" {
			fir.Spec.Release = fib.Spec.Release
		}
		if fir.Spec.Namespace == "" {
			fir.Spec.Namespace = fib.Spec.Namespace
		}
		if fir.Spec.SourceCluster == "" {
			fir.Spec.SourceCluster = fib.Spec.Cluster
		}
		return r.Update(ctx, fir)
	}
	var list fleetv1alpha1.FleetInstanceBackupList
	if err := r.List(ctx, &list, client.InNamespace(fir.Namespace)); err != nil {
		return err
	}
	for _, fib := range list.Items {
		if fib.Status.VeleroBackupName == fir.Spec.BackupRef || fib.Name == fir.Spec.BackupRef {
			fir.Spec.BackupCRName = fib.Name
			if fir.Spec.VeleroBackupName == "" {
				fir.Spec.VeleroBackupName = fib.Status.VeleroBackupName
			}
			if fir.Spec.InstanceID == "" {
				fir.Spec.InstanceID = fib.Spec.InstanceID
			}
			if fir.Spec.Release == "" {
				fir.Spec.Release = fib.Spec.Release
			}
			if fir.Spec.Namespace == "" {
				fir.Spec.Namespace = fib.Spec.Namespace
			}
			if fir.Spec.SourceCluster == "" {
				fir.Spec.SourceCluster = fib.Spec.Cluster
			}
			return r.Update(ctx, fir)
		}
	}
	if fir.Spec.VeleroBackupName == "" {
		fir.Spec.VeleroBackupName = fir.Spec.BackupRef
	}
	if fir.Spec.VeleroBackupName == "" {
		return fmt.Errorf("backup ref %q not resolved", fir.Spec.BackupRef)
	}
	return nil
}

func (r *InstanceRestoreReconciler) rebindDCMInstance(ctx context.Context, fir *fleetv1alpha1.FleetInstanceRestore) (string, error) {
	secretName := backup.BackupSecretName(fir.Spec.BackupCRName)
	if fir.Spec.BackupCRName == "" {
		return "", fmt.Errorf("backup CR name required for rebind")
	}
	var secret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Namespace: fir.Namespace, Name: secretName}, &secret); err != nil {
		return "", err
	}
	raw := secret.Data["dcm-instance.json"]
	if len(raw) == 0 {
		return "", fmt.Errorf("dcm-instance.json missing in secret %s", secretName)
	}
	var inst dcm.CatalogItemInstance
	if err := json.Unmarshal(raw, &inst); err != nil {
		return "", err
	}
	patches := []dcm.UserValue{dcm.UserValueFromString("routing.cluster", fir.Spec.TargetCluster)}
	if fir.Spec.Rebind != nil {
		for _, p := range fir.Spec.Rebind.UserValuePatches {
			patches = append(patches, dcm.UserValueFromString(p.Path, p.Value))
		}
	}
	userValues := workload.ApplyUserValuePatches(inst.Spec.UserValues, patches)
	mode := rebindMode(fir.Spec.Rebind)
	suffix := defaultCloneSuffix
	if fir.Spec.Rebind != nil && fir.Spec.Rebind.DisplayNameSuffix != "" {
		suffix = fir.Spec.Rebind.DisplayNameSuffix
	}
	displayName := resolveRebindDisplayName(mode, inst.DisplayName, suffix)
	dcmClient := dcm.NewClient(r.Cfg.DCMCatalogAPIURL)
	if mode == fleetv1alpha1.RebindModeSupersede {
		catalog, err := dcmClient.ListCatalogItemInstances(ctx)
		if err != nil {
			return "", fmt.Errorf("list catalog instances: %w", err)
		}
		sourceIDs := resolveSupersedeInstanceIDs(fir.Spec.InstanceID, inst.DisplayName, catalog)
		if len(sourceIDs) == 0 {
			return "", fmt.Errorf("instanceId required for supersede rebind")
		}
		for _, sourceID := range sourceIDs {
			if err := deleteFleetAppRoutesForInstance(ctx, r.Client, r.Cfg.EdgeRoutesNamespace, sourceID); err != nil {
				return "", fmt.Errorf("delete fleet app routes for %s: %w", sourceID, err)
			}
			if err := retryRebind(ctx, 5, 3*time.Second, func() error {
				return dcmClient.DeleteCatalogItemInstance(ctx, sourceID, "abandon")
			}); err != nil {
				return "", fmt.Errorf("delete catalog instance %s: %w", sourceID, err)
			}
		}
	}
	var resp *dcm.CreateCatalogItemInstanceResponse
	if err := retryRebind(ctx, 5, 3*time.Second, func() error {
		var err error
		resp, err = dcmClient.CreateCatalogItemInstance(ctx, dcm.CreateCatalogItemInstanceRequest{
			DisplayName: displayName,
			Spec: dcm.CatalogItemInstanceSpec{
				CatalogItemID: inst.Spec.CatalogItemID,
				UserValues:    userValues,
				LabelSelector: inst.Spec.LabelSelector,
			},
		})
		return err
	}); err != nil {
		return "", err
	}
	if resp.ID != "" {
		return resp.ID, nil
	}
	return resp.UID, nil
}

func retryRebind(ctx context.Context, attempts int, wait time.Duration, fn func() error) error {
	var last error
	for i := 0; i < attempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			last = err
		}
		if i+1 < attempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
	}
	return last
}

func (r *InstanceRestoreReconciler) completeRestore(ctx context.Context, fir *fleetv1alpha1.FleetInstanceRestore, newID string) (ctrl.Result, error) {
	fir.Status.Phase = fleetv1alpha1.RestorePhaseComplete
	fir.Status.Message = "Restore complete"
	fir.Status.NewInstanceID = newID
	fir.Status.CompletedAt = backup.NowMeta()
	fir.Status.ObservedGeneration = fir.Generation
	if err := r.Status().Update(ctx, fir); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *InstanceRestoreReconciler) failRestore(ctx context.Context, fir *fleetv1alpha1.FleetInstanceRestore, err error) (ctrl.Result, error) {
	fir.Status.Phase = fleetv1alpha1.RestorePhaseFailed
	fir.Status.Message = err.Error()
	fir.Status.CompletedAt = backup.NowMeta()
	_ = r.Status().Update(ctx, fir)
	return ctrl.Result{}, nil
}

func defaultNS(ns string) string {
	if strings.TrimSpace(ns) == "" {
		return "default"
	}
	return ns
}

func (r *Reconciler) SetupInstanceBackupWithManager(mgr ctrl.Manager) error {
	ibr := &InstanceBackupReconciler{Reconciler: r}
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&fleetv1alpha1.FleetInstanceBackup{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Secret{}).
		Complete(ibr); err != nil {
		return err
	}
	irr := &InstanceRestoreReconciler{Reconciler: r}
	return ctrl.NewControllerManagedBy(mgr).
		For(&fleetv1alpha1.FleetInstanceRestore{}).
		Owns(&batchv1.Job{}).
		Complete(irr)
}

// ensureBackupInventoryCM is on InstanceBackupReconciler; expose for restore via embedding.
func (r *InstanceRestoreReconciler) ensureBackupInventoryCM(ctx context.Context) error {
	ibr := &InstanceBackupReconciler{Reconciler: r.Reconciler}
	return ibr.ensureBackupInventoryCM(ctx)
}
