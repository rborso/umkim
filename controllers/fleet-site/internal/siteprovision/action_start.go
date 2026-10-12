package siteprovision

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/provision"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitehosts"
	"github.com/rborso/umkim/controllers/fleet-site/internal/siteinventory"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StartYaidapActionOptions configures a YAIDAP connector action Job.
type StartYaidapActionOptions struct {
	// Params mirrors the HTTP action body params (tags/playbook overrides).
	Params map[string]interface{}
	// GuestFound reports whether the subject is a known inventory host.
	GuestFound bool
	// HubFleetSSHPublicKey is injected as fleet_ansible_ssh_pubkey for create/install.
	HubFleetSSHPublicKey string
	ActorKind            string
	ActorName            string
}

// StartYaidapAction creates the YAIDAP connector Job for a site provision action.
// Shared by the HTTP API and the controller-driven bring-up pipeline so both run
// the same pod-based flow.
func StartYaidapAction(ctx context.Context, c client.Client, cfg config.Config, fs *fleetv1alpha1.FleetSite, rec *fleetv1alpha1.SiteProvisionActionRecord, opts StartYaidapActionOptions) (string, error) {
	spec := fs.Spec.Provision
	inv, err := provision.DefaultYaidapInvocation(rec.Action, rec.Confirm)
	if err != nil {
		return "", err
	}
	if tags, ok := opts.Params["tags"].([]interface{}); ok && len(tags) > 0 {
		inv.Tags = nil
		for _, t := range tags {
			if s, ok := t.(string); ok && strings.TrimSpace(s) != "" {
				inv.Tags = append(inv.Tags, strings.TrimSpace(s))
			}
		}
	}
	if pb, ok := opts.Params["playbook"].(string); ok && strings.TrimSpace(pb) != "" {
		inv.Playbook = strings.TrimSpace(pb)
	}

	cmName, err := siteinventory.EnsureJumphostInventoryCM(ctx, c, cfg, fs)
	if err != nil {
		return "", err
	}

	yaidapRoot := strings.TrimSpace(spec.YaidaPRoot)
	if yaidapRoot == "" {
		yaidapRoot = "~/yaidap"
	}
	extra := []string{
		"-e", "fleet_provision_action=" + rec.Action,
		"-e", "fleet_provision_subject=" + rec.Subject,
		"-e", fmt.Sprintf("fleet_provision_confirm=%t", rec.Confirm),
		"-e", "fleet_provision_inventory_path=" + spec.InventoryPath,
		"-e", "fleet_provision_yaidap_root=" + yaidapRoot,
		"-e", "fleet_provision_playbook=" + inv.Playbook,
		"-e", "fleet_provision_tags=" + strings.Join(inv.Tags, ","),
	}
	concreteVmLocation := false
	if rec.Action == fleetv1alpha1.ProvisionActionCreateVM && opts.GuestFound {
		invHost, _, guestErr := yaidap.LookupInventoryHost(ctx, c, cfg.Namespace, fs.Name, rec.Subject)
		if guestErr == nil {
			loc := strings.TrimSpace(invHost.VmLocation)
			if loc == "" {
				loc = strings.TrimSpace(invHost.Status.VmLocation)
			}
			concreteVmLocation = !sitehosts.VmLocationIsScheduleToken(loc)
		}
	}
	// YAIDAP nested playbook extras (confirm_destroy, yaidap_emit_result, etc.) are resolved
	// in site-provision-connector.yml from fleet_provision_action / fleet_provision_confirm.
	if concreteVmLocation {
		extra = append(extra, "-e", "fleet_provision_yaidap_recreate=true")
	}
	if provision.ActionUsesConnectionLocal(rec.Action) {
		extra = append(extra, "-e", "fleet_provision_connection_local=true")
	}
	if rec.Action == fleetv1alpha1.ProvisionActionCreateVM || rec.Action == fleetv1alpha1.ProvisionActionInstallBM {
		if pub := strings.TrimSpace(opts.HubFleetSSHPublicKey); pub != "" {
			extra = append(extra, "-e", fmt.Sprintf("fleet_provision_fleet_ansible_ssh_pubkey=%q", pub))
		}
	}
	for k, v := range spec.DefaultExtraVars {
		if strings.TrimSpace(k) == "" {
			continue
		}
		extra = append(extra, "-e", k+"="+v)
	}
	if provision.ActionSyncsYaidapInventory(rec.Action) && (opts.GuestFound || rec.Action == fleetv1alpha1.ProvisionActionInstallBM) {
		actor := connectors.FormatActor(opts.ActorKind, opts.ActorName)
		if err := yaidap.EnsureReadyForProvision(ctx, c, cfg.Namespace, fs.Name, actor); err != nil {
			return "", err
		}
		bundle, err := yaidap.ProjectFromStore(ctx, c, cfg.Namespace, fs.Name)
		if err != nil {
			return "", err
		}
		syncExtra, err := yaidap.ProvisionPreSyncExtraVars(bundle)
		if err != nil {
			return "", err
		}
		extra = append(extra, syncExtra...)
	}

	jobName := "fleet-site-prov-" + rec.ID
	var old batchv1.Job
	if err := c.Get(ctx, types.NamespacedName{Namespace: cfg.Namespace, Name: jobName}, &old); err == nil {
		_ = c.Delete(ctx, &old, client.PropagationPolicy(metav1.DeletePropagationBackground))
	}
	j := jobs.SiteProvisionConnectorJob(
		cfg.Namespace, jobName, fs.Name, fs.Spec.EffectiveSiteID(fs.Name),
		cfg.AnsibleRunnerImage, spec.SSHSecretRef, cmName, "", extra,
	)
	if err := c.Create(ctx, j); err != nil {
		return "", err
	}
	return jobName, nil
}
