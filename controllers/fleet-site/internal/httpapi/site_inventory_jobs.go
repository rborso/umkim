package httpapi

import (
	"context"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/siteinventory"
)

func (s *Server) ensureJumphostInventoryCM(ctx context.Context, fs *fleetv1alpha1.FleetSite) (string, error) {
	return siteinventory.EnsureJumphostInventoryCM(ctx, s.Client, s.Cfg, fs)
}

func (s *Server) startInventorySyncJob(ctx context.Context, fs *fleetv1alpha1.FleetSite) (string, error) {
	return siteinventory.EnqueueInventorySync(ctx, s.Client, s.Cfg, fs)
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
	return siteinventory.CreateInventoryConnectorJob(ctx, s.Client, s.Cfg, fs, extra, cmName)
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
	return siteinventory.CreateInventoryConnectorJob(ctx, s.Client, s.Cfg, fs, extra, cmName)
}

func (s *Server) createInventoryConnectorJob(ctx context.Context, fs *fleetv1alpha1.FleetSite, extra []string, cmName string) (string, error) {
	return siteinventory.CreateInventoryConnectorJob(ctx, s.Client, s.Cfg, fs, extra, cmName)
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
