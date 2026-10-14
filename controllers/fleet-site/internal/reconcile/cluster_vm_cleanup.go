package reconcile

import (
	"context"
	"fmt"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/siteprovision"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sites"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	annotationCleanupVms     = "fleet.umkim.com/cleanup-vms"
	annotationCleanupVmDecls = "fleet.umkim.com/cleanup-vm-declarations"

	// vmCleanupMaxAttempts bounds destroy retries per guest before the cluster
	// delete gives up on it (the guest is reported and left in inventory) so a
	// broken hypervisor cannot wedge the fleet.
	vmCleanupMaxAttempts = 2
)

func clusterCleanupVms(fcj *fleetv1alpha1.FleetClusterJoin) bool {
	return fcj.Annotations != nil && fcj.Annotations[annotationCleanupVms] == "true"
}

func clusterCleanupVmDeclarations(fcj *fleetv1alpha1.FleetClusterJoin) bool {
	return fcj.Annotations != nil && fcj.Annotations[annotationCleanupVmDecls] == "true"
}

// reconcileClusterDeleteVms destroys the YAIDAP VM guests backing the cluster —
// and, when asked, removes their site inventory declarations — as the final stage
// of cluster delete. The destroy guard already allows a guest whose join is
// Deleting, so this runs while the FleetClusterJoin is still present.
//
// One connector action runs per site at a time, so guests are destroyed
// sequentially; progress lives on the FleetClusterJoin status so a requeue (or a
// hub restart) resumes instead of restarting. done=false means "requeue after
// wait"; a non-zero wait always accompanies it.
func (r *ClusterJoinReconciler) reconcileClusterDeleteVms(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin) (done bool, wait time.Duration, err error) {
	if !clusterCleanupVms(fcj) {
		return true, 0, nil
	}
	st := fcj.Status.VmCleanup
	if st != nil && (st.Phase == fleetv1alpha1.VmCleanupPhaseDone || st.Phase == fleetv1alpha1.VmCleanupPhaseSkipped) {
		return true, 0, nil
	}
	waitAfter := time.Duration(r.Cfg.RequeueIntervalSec) * time.Second
	if waitAfter <= 0 {
		waitAfter = 15 * time.Second
	}
	removeDecls := clusterCleanupVmDeclarations(fcj)
	save := func() error {
		fcj.Status.VmCleanup = st
		return r.Status().Update(ctx, fcj)
	}

	site, err := sites.FindFleetSite(ctx, r.Client, r.Cfg.Namespace, fcj.Spec.SiteId)
	if err != nil {
		return true, 0, r.setVmCleanupSkipped(ctx, fcj, "site "+fcj.Spec.SiteId+" not found; skipping VM cleanup")
	}
	if strings.TrimSpace(site.Spec.EffectiveProvisionProvider()) != fleetv1alpha1.ProvisionProviderYaidap {
		return true, 0, r.setVmCleanupSkipped(ctx, fcj, "site "+site.Name+" has no YAIDAP provisioner; skipping VM cleanup")
	}

	// Build the work list once, from the cluster's nodes. Only VM guests found in
	// the site inventory are destroyable; bare metal, hypervisors and external
	// hosts have no libvirt domain to remove.
	if st == nil || st.Phase == "" {
		hosts, err := r.vmCleanupHosts(ctx, site.Name, fcj)
		if err != nil {
			return false, 0, err
		}
		st = &fleetv1alpha1.ClusterVmCleanupStatus{Phase: fleetv1alpha1.VmCleanupPhaseDestroying, Hosts: hosts}
		if len(hosts) == 0 {
			st.Phase = fleetv1alpha1.VmCleanupPhaseDone
			st.Message = "no YAIDAP VM guests to destroy"
			return true, 0, save()
		}
	}

	for _, host := range st.Hosts {
		if vmCleanupCompleted(st, host) {
			continue
		}
		view, found, err := yaidap.LookupInventoryHost(ctx, r.Client, r.Cfg.Namespace, site.Name, host)
		if err != nil {
			return false, 0, err
		}
		kind := strings.TrimSpace(view.Kind)
		if kind == "" {
			kind = fleetv1alpha1.SiteHostKindVM
		}
		if !found || view.External || kind != fleetv1alpha1.SiteHostKindVM {
			// Not a destroyable VM guest (bare metal, hypervisor, gone from the
			// inventory). A cluster node with no inventory entry at all is worth
			// flagging: its VM can only be found on the hypervisor.
			if !found && !view.External {
				recordVmCleanupFailure(st, host, "not in the site inventory; check the hypervisor")
			}
			vmCleanupFinish(st, host)
			continue
		}
		phase := r.guestRecordedPhase(ctx, site.Name, host)

		// An in-flight destroy is polled before the guest phase is considered: the
		// connector marks the guest Destroying for the whole action, so the phase
		// alone cannot tell "waiting for our job" from "someone else is working".
		if st.JobName != "" {
			done, ok, msg := r.jobDoneAllowMissing(ctx, fcj.Namespace, st.JobName, false)
			if !done {
				st.Message = fmt.Sprintf("destroying %s (%s)", host, st.JobName)
				return false, 15 * time.Second, save()
			}
			st.JobName, st.ActionID = "", ""
			if ok {
				if err := r.finishVmCleanupHost(ctx, fcj, st, host, removeDecls); err != nil {
					return false, 30 * time.Second, save()
				}
				continue
			}
			if phase == fleetv1alpha1.SiteHostPhaseDeclared {
				// The recorded phase said there was no VM to destroy; the failure
				// confirms that. Keep the declaration: the phase has been wrong
				// before, and dropping it would lose the only pointer to a VM that
				// might still be running.
				st.Message = fmt.Sprintf("%s reported no VM to destroy; declaration kept", host)
				vmCleanupFinish(st, host)
				continue
			}
			attempts := vmCleanupAttempts(st, host) + 1
			recordVmCleanupFailure(st, host, msg)
			if attempts < vmCleanupMaxAttempts {
				st.Message = fmt.Sprintf("destroy of %s failed (%s); retrying (%d/%d)", host, msg, attempts, vmCleanupMaxAttempts)
				return false, 30 * time.Second, save()
			}
			// Give up on this guest but finish the delete: the fleet must not hold
			// an undeletable cluster because a hypervisor is down.
			st.Message = fmt.Sprintf("destroy of %s failed (%s); leaving it in inventory", host, msg)
			vmCleanupMarkDone(st, host)
			continue
		}

		if phase == fleetv1alpha1.SiteHostPhaseProvisioning || phase == fleetv1alpha1.SiteHostPhaseDestroying {
			st.Message = fmt.Sprintf("waiting for %s (%s)", host, phase)
			return false, waitAfter, save()
		}

		// Destroy regardless of the recorded phase — Ready, Failed, Declared or
		// unknown. The phase is only a hint: it has read Declared for a VM that was
		// still running (stale status), and a skipped destroy leaves that VM behind,
		// which is the one outcome this cleanup must not have. A destroy against a
		// guest that really is gone either no-ops or fails harmlessly (handled
		// above, where the declaration is kept).

		fs := site.DeepCopy()
		if siteprovision.ActionBusy(fs) {
			st.Message = fmt.Sprintf("waiting for another site action before destroying %s", host)
			return false, waitAfter, save()
		}
		actionID, jobName, err := r.startVmCleanupDestroy(ctx, fs, host)
		if err != nil {
			return false, 0, err
		}
		st.ActionID = actionID
		st.JobName = jobName
		st.Message = fmt.Sprintf("destroying %s (%s)", host, jobName)
		return false, waitAfter, save()
	}

	st.Phase = fleetv1alpha1.VmCleanupPhaseDone
	if len(st.Failures) > 0 {
		st.Phase = fleetv1alpha1.VmCleanupPhaseFailed
	}
	st.Message = vmCleanupDoneMessage(st, removeDecls)
	return true, 0, save()
}

// guestRecordedPhase returns the phase actually recorded for a guest. The
// inventory host view substitutes Declared for a guest with no status record,
// which is exactly the wrong answer when deciding whether a libvirt domain still
// exists — so the cleanup reads the raw value and treats "" as unknown.
func (r *ClusterJoinReconciler) guestRecordedPhase(ctx context.Context, siteName, host string) string {
	doc, err := yaidap.LoadStatusDoc(ctx, r.Client, r.Cfg.Namespace, siteName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(doc.Guests[host].Phase)
}

// vmCleanupHosts lists the cluster's nodes that are YAIDAP VM guests.
func (r *ClusterJoinReconciler) vmCleanupHosts(ctx context.Context, siteName string, fcj *fleetv1alpha1.FleetClusterJoin) ([]string, error) {
	hosts := make([]string, 0, len(fcj.Spec.Nodes))
	seen := map[string]bool{}
	for _, node := range fcj.Spec.Nodes {
		host := strings.TrimSpace(node.Hostname)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		view, found, err := yaidap.LookupInventoryHost(ctx, r.Client, r.Cfg.Namespace, siteName, host)
		if err != nil {
			return nil, err
		}
		if !found || view.External {
			continue
		}
		kind := strings.TrimSpace(view.Kind)
		if kind == "" {
			kind = fleetv1alpha1.SiteHostKindVM
		}
		if kind != fleetv1alpha1.SiteHostKindVM {
			continue
		}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

// startVmCleanupDestroy runs one destroy action for a guest through the shared
// site provision action (a connector pod), recorded in the site history so the
// console shows it next to the cluster's other operations. Returns the action id
// and the connector job name.
func (r *ClusterJoinReconciler) startVmCleanupDestroy(ctx context.Context, site *fleetv1alpha1.FleetSite, host string) (string, string, error) {
	now := metav1.NewTime(time.Now().UTC())
	rec := fleetv1alpha1.SiteProvisionActionRecord{
		ID:        newActionID(),
		Action:    fleetv1alpha1.ProvisionActionDestroy,
		Subject:   host,
		Provider:  fleetv1alpha1.ProvisionProviderYaidap,
		Phase:     fleetv1alpha1.ProvisionPhasePending,
		Confirm:   true,
		ActorKind: "Controller",
		ActorName: "cluster-delete",
		StartedAt: &now,
	}
	jobName, err := siteprovision.StartYaidapAction(ctx, r.Client, r.Cfg, site, &rec, siteprovision.StartYaidapActionOptions{
		GuestFound: true,
		ActorKind:  "Controller",
		ActorName:  "cluster-delete",
	})
	if err != nil {
		return "", "", err
	}
	rec.Phase = fleetv1alpha1.ProvisionPhaseRunning
	rec.Message = "YAIDAP destroy job started"
	rec.JobName = jobName
	if err := siteprovision.CommitAction(ctx, r.Client, site, rec, false); err != nil {
		return "", "", err
	}
	return rec.ID, jobName, nil
}

// finishVmCleanupHost removes the guest's inventory declaration (when asked) and
// marks it done. The declaration is only dropped once its VM is gone.
func (r *ClusterJoinReconciler) finishVmCleanupHost(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, st *fleetv1alpha1.ClusterVmCleanupStatus, host string, removeDecls bool) error {
	if removeDecls {
		if err := yaidap.RemoveHostDeclaration(ctx, r.Client, r.Cfg.Namespace, fcj.Spec.SiteId, host); err != nil {
			st.Message = fmt.Sprintf("removing declaration of %s failed: %v", host, err)
			return err
		}
		// Also drop the host from the cluster group file, which the pipeline
		// created when it declared the guest.
		clusterName := fcj.Spec.EffectiveClusterName(fcj.Name)
		if err := yaidap.RemoveClusterGroupMembers(ctx, r.Client, r.Cfg.Namespace, fcj.Spec.SiteId, clusterName, []string{host}); err != nil {
			st.Message = fmt.Sprintf("removing %s from cluster group %s failed: %v", host, clusterName, err)
			return err
		}
	}
	vmCleanupFinish(st, host)
	return nil
}

// vmCleanupFinish marks a guest done and forgets its earlier attempt failures.
func vmCleanupFinish(st *fleetv1alpha1.ClusterVmCleanupStatus, host string) {
	vmCleanupMarkDone(st, host)
	kept := st.Failures[:0]
	for _, f := range st.Failures {
		if !strings.HasPrefix(f, host+": ") {
			kept = append(kept, f)
		}
	}
	st.Failures = kept
}

// vmCleanupMarkDone records a guest as finished without touching its failure
// history, so a guest we gave up on still shows why.
func vmCleanupMarkDone(st *fleetv1alpha1.ClusterVmCleanupStatus, host string) {
	if vmCleanupCompleted(st, host) {
		return
	}
	st.Completed = append(st.Completed, host)
}

func vmCleanupDoneMessage(st *fleetv1alpha1.ClusterVmCleanupStatus, removeDecls bool) string {
	if len(st.Failures) > 0 {
		return fmt.Sprintf("VM cleanup finished with %d failure(s): %s", len(st.Failures), strings.Join(st.Failures, "; "))
	}
	verb := "destroyed"
	if removeDecls {
		verb = "destroyed and removed from inventory"
	}
	return fmt.Sprintf("%d VM guest(s) %s", len(st.Completed), verb)
}

func vmCleanupCompleted(st *fleetv1alpha1.ClusterVmCleanupStatus, host string) bool {
	for _, h := range st.Completed {
		if h == host {
			return true
		}
	}
	return false
}

func vmCleanupAttempts(st *fleetv1alpha1.ClusterVmCleanupStatus, host string) int {
	n := 0
	for _, f := range st.Failures {
		if strings.HasPrefix(f, host+": ") {
			n++
		}
	}
	return n
}

func recordVmCleanupFailure(st *fleetv1alpha1.ClusterVmCleanupStatus, host, msg string) {
	if strings.TrimSpace(msg) == "" {
		msg = "destroy job failed"
	}
	st.Failures = append(st.Failures, host+": "+msg)
}

func (r *ClusterJoinReconciler) setVmCleanupSkipped(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, msg string) error {
	fcj.Status.VmCleanup = &fleetv1alpha1.ClusterVmCleanupStatus{
		Phase:   fleetv1alpha1.VmCleanupPhaseSkipped,
		Message: msg,
	}
	return r.Status().Update(ctx, fcj)
}

// vmCleanupMessage renders the delete-stage status line while VM cleanup is in
// flight, so the console and CLI show which guest is being destroyed.
func vmCleanupMessage(fcj *fleetv1alpha1.FleetClusterJoin) string {
	st := fcj.Status.VmCleanup
	if st == nil {
		return "Destroying cluster VMs"
	}
	msg := fmt.Sprintf("Destroying cluster VMs (%d/%d)", len(st.Completed), len(st.Hosts))
	if strings.TrimSpace(st.Message) != "" {
		msg += ": " + st.Message
	}
	return msg
}
