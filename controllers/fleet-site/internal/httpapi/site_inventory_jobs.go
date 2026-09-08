package httpapi

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const siteInventoryJobPrefix = "fleet-site-inv-"

func (s *Server) ensureJumphostInventoryCM(ctx context.Context, fs *fleetv1alpha1.FleetSite) (string, error) {
	spec := fs.Spec.Provision
	cmName := fmt.Sprintf(siteProvisionInvCMFmt, fs.Name)
	ini := jumphostInventoryINI(spec.Jumphost, s.Cfg.SSHUser)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: s.Cfg.Namespace,
			Labels:    map[string]string{"app": "fleet-site-provision", "fleet-site": fs.Name},
		},
		Data: map[string]string{"inventory.ini": ini},
	}
	var existing corev1.ConfigMap
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: cmName}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			if err := s.Client.Create(ctx, cm); err != nil {
				return "", err
			}
			return cmName, nil
		}
		return "", err
	}
	existing.Data = cm.Data
	if err := s.Client.Update(ctx, &existing); err != nil {
		return "", err
	}
	return cmName, nil
}

func (s *Server) startInventorySyncJob(ctx context.Context, fs *fleetv1alpha1.FleetSite) (string, error) {
	spec := fs.Spec.Provision
	bundle, err := yaidap.ProjectFromStore(ctx, s.Client, s.Cfg.Namespace, fs.Name)
	if err != nil {
		return "", err
	}
	syncExtra, err := yaidap.SyncExtraVars(bundle)
	if err != nil {
		return "", err
	}
	cmName, err := s.ensureJumphostInventoryCM(ctx, fs)
	if err != nil {
		return "", err
	}
	yaidapRoot := strings.TrimSpace(spec.YaidaPRoot)
	if yaidapRoot == "" {
		yaidapRoot = "~/yaidap"
	}
	extra := []string{
		"-e", "fleet_provision_subject=inventory",
		"-e", "fleet_provision_inventory_path=" + spec.InventoryPath,
		"-e", "fleet_provision_yaidap_root=" + yaidapRoot,
		"-e", "fleet_provision_playbook=site-provision-connector.yml",
	}
	extra = append(extra, syncExtra...)
	return s.createInventoryConnectorJob(ctx, fs, extra, cmName)
}

func (s *Server) startInventoryImportPreviewJob(ctx context.Context, fs *fleetv1alpha1.FleetSite) (string, error) {
	spec := fs.Spec.Provision
	cmName, err := s.ensureJumphostInventoryCM(ctx, fs)
	if err != nil {
		return "", err
	}
	yaidapRoot := strings.TrimSpace(spec.YaidaPRoot)
	if yaidapRoot == "" {
		yaidapRoot = "~/yaidap"
	}
	extra := []string{
		"-e", "fleet_provision_subject=inventory",
		"-e", "fleet_provision_inventory_path=" + spec.InventoryPath,
		"-e", "fleet_provision_yaidap_root=" + yaidapRoot,
		"-e", "fleet_provision_playbook=site-provision-connector.yml",
	}
	extra = append(extra, yaidap.ImportPreviewExtraVars(spec.InventoryPath)...)
	return s.createInventoryConnectorJob(ctx, fs, extra, cmName)
}

func (s *Server) startInventoryImportJob(ctx context.Context, fs *fleetv1alpha1.FleetSite) (string, error) {
	spec := fs.Spec.Provision
	cmName, err := s.ensureJumphostInventoryCM(ctx, fs)
	if err != nil {
		return "", err
	}
	yaidapRoot := strings.TrimSpace(spec.YaidaPRoot)
	if yaidapRoot == "" {
		yaidapRoot = "~/yaidap"
	}
	extra := []string{
		"-e", "fleet_provision_subject=inventory",
		"-e", "fleet_provision_inventory_path=" + spec.InventoryPath,
		"-e", "fleet_provision_yaidap_root=" + yaidapRoot,
		"-e", "fleet_provision_playbook=site-provision-connector.yml",
	}
	extra = append(extra, yaidap.ImportExtraVars(spec.InventoryPath)...)
	return s.createInventoryConnectorJob(ctx, fs, extra, cmName)
}

func (s *Server) createInventoryConnectorJob(ctx context.Context, fs *fleetv1alpha1.FleetSite, extra []string, cmName string) (string, error) {
	spec := fs.Spec.Provision
	jobName := siteInventoryJobPrefix + newProvisionActionID()
	var old batchv1.Job
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: jobName}, &old); err == nil {
		_ = s.Client.Delete(ctx, &old, client.PropagationPolicy(metav1.DeletePropagationBackground))
	}
	j := jobs.SiteProvisionConnectorJob(
		s.Cfg.Namespace, jobName, fs.Name, fs.Spec.EffectiveSiteID(fs.Name),
		s.Cfg.AnsibleRunnerImage, spec.SSHSecretRef, cmName, extra,
	)
	if err := s.Client.Create(ctx, j); err != nil {
		return "", err
	}
	return jobName, nil
}

// yaidapProvisionInventoryExtra ensures connector store is projected before YAIDAP actions.
func (s *Server) yaidapProvisionInventoryExtra(ctx context.Context, fs *fleetv1alpha1.FleetSite, subject, actorKind, actorName string) ([]string, error) {
	actor := connectorsFormatActor(actorKind, actorName)
	if err := yaidap.EnsureReadyForProvision(ctx, s.Client, s.Cfg.Namespace, fs.Name, actor); err != nil {
		return nil, err
	}
	bundle, err := yaidap.ProjectFromStore(ctx, s.Client, s.Cfg.Namespace, fs.Name)
	if err != nil {
		return nil, err
	}
	return yaidap.ProvisionPreSyncExtraVars(bundle)
}

func connectorsFormatActor(kind, name string) string {
	return connectors.FormatActor(kind, name)
}
