package siteprovision

import (
	"context"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ansiblelogs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/provision"
	"github.com/rborso/umkim/controllers/fleet-site/internal/siteinventory"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	maxProvisionHistory = 20
	maxProvisionLogTail   = 65536
)

// RefreshRunningJob polls the connector Job for a site with a running provision action,
// finalizes FleetSite status, and syncs hub host inventory.
// cs is optional; when set, create_vm success may read resolved vm_location from job logs.
// cfg is optional; when set, resolved vm_location triggers an inventory sync job.
func RefreshRunningJob(ctx context.Context, c client.Client, ns string, fs *fleetv1alpha1.FleetSite, cs kubernetes.Interface, cfg *config.Config) error {
	if fs.Status.Provision == nil || fs.Status.Provision.Current == nil {
		return nil
	}
	cur := fs.Status.Provision.Current
	if cur.Phase != fleetv1alpha1.ProvisionPhaseRunning && cur.Phase != fleetv1alpha1.ProvisionPhasePending {
		return nil
	}
	if cur.JobName == "" {
		return nil
	}
	var job batchv1.Job
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: cur.JobName}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			end := metav1.NewTime(time.Now().UTC())
			cur.Phase = fleetv1alpha1.ProvisionPhaseFailed
			cur.Message = "provision job missing"
			cur.EndedAt = &end
			return CommitAction(ctx, c, fs, *cur, true)
		}
		return err
	}
	if job.Status.Succeeded > 0 {
		end := metav1.NewTime(time.Now().UTC())
		cur.Phase = fleetv1alpha1.ProvisionPhaseSucceeded
		cur.Message = "YAIDAP connector job succeeded"
		if cur.Action == fleetv1alpha1.ProvisionActionPlanVM {
			cur.Message = "YAIDAP placement plan succeeded (see job logs for assignments)"
		}
		if msg := yaidap.DiscoverySuccessMessage(cur.Action); msg != "" {
			cur.Message = msg
		}
		if cur.Action == fleetv1alpha1.ProvisionActionDestroy {
			cur.Message = yaidap.DestroySuccessMessage()
		}
		cur.EndedAt = &end
		if cs != nil && strings.TrimSpace(cur.JobName) != "" {
			if logs, err := ansiblelogs.PodLogs(ctx, cs, ns, cur.JobName); err == nil {
				cur.LogTail = ansiblelogs.Tail(logs, maxProvisionLogTail)
				applyYaidapJobResult(ctx, c, ns, fs, cur, logs, cfg)
			}
		} else {
			enrichHandoffHost(ctx, c, ns, fs.Name, cur)
		}
		if err := CommitAction(ctx, c, fs, *cur, true); err != nil {
			return err
		}
		return nil
	}
	if job.Status.Failed > 0 {
		end := metav1.NewTime(time.Now().UTC())
		cur.Phase = fleetv1alpha1.ProvisionPhaseFailed
		cur.Message = "YAIDAP connector job failed"
		cur.EndedAt = &end
		if cs != nil && strings.TrimSpace(cur.JobName) != "" {
			if logs, err := ansiblelogs.PodLogs(ctx, cs, ns, cur.JobName); err == nil {
				cur.LogTail = ansiblelogs.Tail(logs, maxProvisionLogTail)
			}
		}
		return CommitAction(ctx, c, fs, *cur, true)
	}
	return nil
}

func applyYaidapJobResult(ctx context.Context, c client.Client, ns string, fs *fleetv1alpha1.FleetSite, cur *fleetv1alpha1.SiteProvisionActionRecord, logs string, cfg *config.Config) {
	if result, ok := provision.ParseYaidapResult(logs); ok && result != nil {
		if msg := strings.TrimSpace(result.Message); msg != "" {
			cur.Message = msg
		}
		if host := result.ToHandoffHost(fs.Name); host != nil {
			cur.Host = host
		}
		if cur.Action == fleetv1alpha1.ProvisionActionPlanVM {
			if summary := provision.FormatPlacementSummary(result.Placement); summary != "" {
				if strings.TrimSpace(cur.Message) == "" {
					cur.Message = summary
				} else {
					cur.Message = strings.TrimSpace(cur.Message) + "; " + summary
				}
			}
		}
		if cur.Action == fleetv1alpha1.ProvisionActionCreateVM {
			if loc := result.ConcreteVmLocation(); loc != "" {
				persistResolvedVmLocation(ctx, c, ns, fs, cur.Subject, loc, cfg)
			}
		}
		if cur.Host == nil || strings.TrimSpace(cur.Host.AnsibleHost) == "" {
			enrichHandoffHost(ctx, c, ns, fs.Name, cur)
		}
	} else {
		if cur.Action == fleetv1alpha1.ProvisionActionCreateVM {
			if loc := provision.ParseResolvedVmLocation(logs); loc != "" {
				persistResolvedVmLocation(ctx, c, ns, fs, cur.Subject, loc, cfg)
			}
		}
		enrichHandoffHost(ctx, c, ns, fs.Name, cur)
	}
	inventoryPath := ""
	if fs.Spec.Provision != nil {
		inventoryPath = fs.Spec.Provision.InventoryPath
	}
	_ = yaidap.FinalizeSyncFromJobLogs(ctx, c, ns, fs.Name, inventoryPath, logs)
}

func persistResolvedVmLocation(ctx context.Context, c client.Client, ns string, fs *fleetv1alpha1.FleetSite, subject, loc string, cfg *config.Config) {
	logger := log.FromContext(ctx)
	if _, err := yaidap.PersistResolvedVmLocation(ctx, c, ns, fs.Name, subject, loc, "system/provision"); err != nil {
		logger.Error(err, "persist resolved vm_location", "guest", subject, "vm_location", loc)
	}
	if err := yaidap.UpdateGuestStatus(ctx, c, ns, fs.Name, subject, yaidap.GuestStatus{
		Phase:      fleetv1alpha1.SiteHostPhaseReady,
		VmLocation: loc,
		Message:    "vm_location=" + loc,
	}); err != nil {
		logger.Error(err, "update guest status after vm_location", "guest", subject)
	}
	if cfg != nil {
		_, _ = siteinventory.EnqueueInventorySync(ctx, c, *cfg, fs)
	}
}

func enrichHandoffHost(ctx context.Context, c client.Client, ns, siteName string, cur *fleetv1alpha1.SiteProvisionActionRecord) {
	if cur.Host == nil {
		cur.Host = &fleetv1alpha1.ProvisionHandoffHost{
			Hostname: cur.Subject,
			SiteID:   siteName,
		}
	}
	if cur.Host.Hostname == "" {
		cur.Host.Hostname = cur.Subject
	}
	if cur.Host.SiteID == "" {
		cur.Host.SiteID = siteName
	}
	if cur.Host.AnsibleHost != "" {
		return
	}
	addr, err := yaidap.GuestAddress(ctx, c, ns, siteName, cur.Subject)
	if err != nil || addr == "" {
		return
	}
	cur.Host.AnsibleHost = addr
	cur.Host.AnsibleUser = "linux"
	cur.Host.OSFamily = "rocky"
}

// CommitAction updates FleetSite provision status and syncs host inventory.
func CommitAction(ctx context.Context, c client.Client, fs *fleetv1alpha1.FleetSite, rec fleetv1alpha1.SiteProvisionActionRecord, finished bool) error {
	key := types.NamespacedName{Namespace: fs.Namespace, Name: fs.Name}
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest fleetv1alpha1.FleetSite
		if err := c.Get(ctx, key, &latest); err != nil {
			return err
		}
		if latest.Status.Provision == nil {
			latest.Status.Provision = &fleetv1alpha1.SiteProvisionStatus{}
		}
		st := latest.Status.Provision
		st.Phase = rec.Phase
		st.Message = rec.Message
		st.LastActionID = rec.ID
		st.LastActionType = rec.Action
		if finished {
			if st.Current != nil && st.Current.ID != rec.ID {
				st.History = prependHistory(st.History, *st.Current)
			}
			st.History = prependHistory(st.History, rec)
			st.Current = nil
		} else {
			if st.Current != nil && st.Current.ID != rec.ID {
				st.History = prependHistory(st.History, *st.Current)
			}
			cp := rec
			st.Current = &cp
		}
		latest.Status.Provision = st
		if err := c.Status().Update(ctx, &latest); err != nil {
			return err
		}
		*fs = latest
		return nil
	})
	if err != nil {
		return err
	}
	SyncHostInventory(ctx, c, fs.Namespace, fs.Name, rec)
	return nil
}

// SyncHostInventory patches guest status from a provision action record.
func SyncHostInventory(ctx context.Context, c client.Client, ns, siteName string, rec fleetv1alpha1.SiteProvisionActionRecord) {
	hostName := rec.Subject
	if hostName == "" {
		return
	}
	// Cluster bring-up steps (bootstrap/provision) are recorded for the site
	// provision history but are not site-host operations; never write guest status
	// for them (their subject is a cluster name).
	if rec.Action == fleetv1alpha1.ProvisionActionBootstrap || rec.Action == fleetv1alpha1.ProvisionActionProvision {
		return
	}
	st := yaidap.GuestStatus{LastProvisionID: rec.ID, Message: rec.Message}
	// Only actions that create the host (or destroy it) may move the guest phase.
	// plan_vm is a dry-run, and discover/status/prepare_* do not provision the
	// guest, so they must leave its phase untouched (UpdateGuestStatus preserves an
	// empty phase). Otherwise a placement plan reports the guest as Ready.
	switch rec.Phase {
	case fleetv1alpha1.ProvisionPhasePending, fleetv1alpha1.ProvisionPhaseRunning:
		switch {
		case rec.Action == fleetv1alpha1.ProvisionActionDestroy:
			st.Phase = fleetv1alpha1.SiteHostPhaseDestroying
		case actionProvisionsHost(rec.Action):
			st.Phase = fleetv1alpha1.SiteHostPhaseProvisioning
		}
	case fleetv1alpha1.ProvisionPhaseSucceeded:
		switch {
		case rec.Action == fleetv1alpha1.ProvisionActionDestroy:
			st.Phase = fleetv1alpha1.SiteHostPhaseDeclared
			st.Message = yaidap.DestroySuccessMessage()
			st.Cluster = &fleetv1alpha1.SiteHostClusterStatus{}
		case actionProvisionsHost(rec.Action):
			st.Phase = fleetv1alpha1.SiteHostPhaseReady
			if rec.Host != nil && rec.Host.AnsibleHost != "" {
				st.AnsibleHost = rec.Host.AnsibleHost
			}
		}
	case fleetv1alpha1.ProvisionPhaseFailed:
		if rec.Action == fleetv1alpha1.ProvisionActionDestroy || actionProvisionsHost(rec.Action) {
			st.Phase = fleetv1alpha1.SiteHostPhaseFailed
		}
	default:
		return
	}
	_ = yaidap.UpdateGuestStatus(ctx, c, ns, siteName, hostName, st)
}

// actionProvisionsHost reports whether a provision action creates the host.
func actionProvisionsHost(action string) bool {
	return action == fleetv1alpha1.ProvisionActionCreateVM || action == fleetv1alpha1.ProvisionActionInstallBM
}

// RepairStaleGuestProvisioning clears hub guest rows left in Provisioning after their connector job succeeded.
func RepairStaleGuestProvisioning(ctx context.Context, c client.Client, ns string, fs *fleetv1alpha1.FleetSite) {
	if fs.Status.Provision == nil {
		return
	}
	if cur := fs.Status.Provision.Current; cur != nil &&
		(cur.Phase == fleetv1alpha1.ProvisionPhasePending || cur.Phase == fleetv1alpha1.ProvisionPhaseRunning) {
		return
	}
	doc, err := yaidap.LoadStatusDoc(ctx, c, ns, fs.Name)
	if err != nil {
		return
	}
	latest := map[string]fleetv1alpha1.SiteProvisionActionRecord{}
	for _, rec := range fs.Status.Provision.History {
		subject := strings.TrimSpace(rec.Subject)
		if subject == "" || rec.Phase != fleetv1alpha1.ProvisionPhaseSucceeded {
			continue
		}
		if prev, ok := latest[subject]; !ok || rec.StartedAt.Time.After(prev.StartedAt.Time) {
			latest[subject] = rec
		}
	}
	for guest, st := range doc.Guests {
		if st.Phase != fleetv1alpha1.SiteHostPhaseProvisioning &&
			st.Phase != fleetv1alpha1.SiteHostPhaseDestroying {
			continue
		}
		rec, ok := latest[guest]
		if !ok {
			continue
		}
		repair := yaidap.GuestStatus{
			LastProvisionID: rec.ID,
		}
		if rec.Action == fleetv1alpha1.ProvisionActionDestroy {
			repair.Phase = fleetv1alpha1.SiteHostPhaseDeclared
			repair.Message = yaidap.DestroySuccessMessage()
			repair.Cluster = &fleetv1alpha1.SiteHostClusterStatus{}
		} else {
			repair.Phase = fleetv1alpha1.SiteHostPhaseReady
			repair.Message = rec.Message
			if rec.Host != nil && strings.TrimSpace(rec.Host.AnsibleHost) != "" {
				repair.AnsibleHost = rec.Host.AnsibleHost
			}
			if strings.TrimSpace(st.VmLocation) != "" {
				repair.VmLocation = st.VmLocation
			}
		}
		_ = yaidap.UpdateGuestStatus(ctx, c, ns, fs.Name, guest, repair)
	}
}

func prependHistory(history []fleetv1alpha1.SiteProvisionActionRecord, rec fleetv1alpha1.SiteProvisionActionRecord) []fleetv1alpha1.SiteProvisionActionRecord {
	out := append([]fleetv1alpha1.SiteProvisionActionRecord{rec}, history...)
	if len(out) > maxProvisionHistory {
		out = out[:maxProvisionHistory]
	}
	return out
}
