package reconcile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ansiblelogs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/clusterbootstrap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/siteprovision"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sites"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// reconcileClusterAutoProvision drives the bring-up pipeline for clusters created
// with spec.autoProvision: create the Declared VM guests (each in a connector pod),
// run the bootstrap bundle via the jumphost (a pod), then provision the hub side.
// Every step is a Job so nothing runs in a client. Returns handled=true when it
// took over the reconcile.
func (r *ClusterJoinReconciler) reconcileClusterAutoProvision(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, bool, error) {
	if !fcj.Spec.AutoProvision {
		return ctrl.Result{}, false, nil
	}
	switch fcj.Status.Phase {
	case "", fleetv1alpha1.JoinPhasePending:
		return r.ensurePipelineHosts(ctx, fcj)
	case fleetv1alpha1.JoinPhaseScriptIssued, fleetv1alpha1.JoinPhaseEnrolled:
		if r.clusterJoinBundleMode(fcj) {
			return r.ensurePipelineBootstrap(ctx, fcj, clusterName)
		}
	case fleetv1alpha1.JoinPhaseValidated:
		if fcj.Annotations == nil || fcj.Annotations["fleet.umkim.com/provision"] != "true" {
			if fcj.Annotations == nil {
				fcj.Annotations = map[string]string{}
			}
			fcj.Annotations["fleet.umkim.com/provision"] = "true"
			if err := r.Update(ctx, fcj); err != nil {
				return ctrl.Result{}, true, err
			}
			return ctrl.Result{Requeue: true}, true, nil
		}
	case fleetv1alpha1.JoinPhaseProvisioning:
		// Surface the cluster provision Job in the site provision history too.
		if fcj.Status.ProvisionJobName != "" && fcj.Status.ProvisionActionID == "" {
			id := pipelineActionID()
			if err := r.commitPipelineAction(ctx, fcj, fleetv1alpha1.ProvisionActionProvision, fcj.Status.ProvisionJobName, id, "Provision cluster (hub finalize)"); err != nil {
				return ctrl.Result{}, true, err
			}
			fcj.Status.ProvisionActionID = id
			if err := r.Status().Update(ctx, fcj); err != nil {
				return ctrl.Result{}, true, err
			}
		}
	case fleetv1alpha1.JoinPhaseFailed:
		return r.resumePipelineAfterFailure(ctx, fcj)
	}
	return ctrl.Result{}, false, nil
}

const (
	pipelineRetriesAnnotation = "fleet.umkim.com/pipeline-retries"
	maxPipelineRetries        = 3
)

// resumePipelineAfterFailure re-runs the step that failed so a transient error
// (hub not yet accepting the cluster, image pull, ACME rate limit) does not wedge
// an auto-provisioned cluster forever. Bounded by an annotation so a genuinely
// broken cluster stops retrying and stays Failed for the operator.
func (r *ClusterJoinReconciler) resumePipelineAfterFailure(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin) (ctrl.Result, bool, error) {
	retries := 0
	if fcj.Annotations != nil {
		retries, _ = strconv.Atoi(strings.TrimSpace(fcj.Annotations[pipelineRetriesAnnotation]))
	}
	if retries >= maxPipelineRetries {
		return ctrl.Result{}, false, nil
	}
	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	fcj.Annotations[pipelineRetriesAnnotation] = strconv.Itoa(retries + 1)
	// Write metadata first: Update rewrites the object from the store, so the
	// status must be set afterwards (it is a subresource).
	if err := r.Update(ctx, fcj); err != nil {
		return ctrl.Result{}, true, err
	}
	switch fcj.Status.PipelineStep {
	case "provision-failed":
		// Back to Validated: the pipeline then re-arms the provision annotation.
		fcj.Status.Phase = fleetv1alpha1.JoinPhaseValidated
		fcj.Status.Message = fmt.Sprintf("Retrying provision (attempt %d/%d)", retries+1, maxPipelineRetries)
	default:
		// Bootstrap failed (or the step is unknown): re-run it from ScriptIssued;
		// the failure handler already cleared BootstrapJobName.
		fcj.Status.Phase = fleetv1alpha1.JoinPhaseScriptIssued
		fcj.Status.Message = fmt.Sprintf("Retrying bootstrap (attempt %d/%d)", retries+1, maxPipelineRetries)
	}
	if err := r.Status().Update(ctx, fcj); err != nil {
		return ctrl.Result{}, true, err
	}
	return ctrl.Result{Requeue: true}, true, nil
}

// commitPipelineAction records a cluster bring-up step (bootstrap/provision) in
// the site provision history so its operation and logs are visible next to the
// host create_vm runs. Callers dedupe with a per-run action ID held on the FCJ
// status; best-effort (the step still runs if the history write fails).
func (r *ClusterJoinReconciler) commitPipelineAction(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, action, jobName, id, message string) error {
	jobName = strings.TrimSpace(jobName)
	if jobName == "" {
		return nil
	}
	site, err := sites.FindFleetSite(ctx, r.Client, r.Cfg.Namespace, fcj.Spec.SiteId)
	if err != nil {
		return nil
	}
	now := metav1.NewTime(time.Now().UTC())
	rec := fleetv1alpha1.SiteProvisionActionRecord{
		ID:        id,
		Action:    action,
		Subject:   fcj.Spec.EffectiveClusterName(fcj.Name),
		Provider:  site.Spec.EffectiveProvisionProvider(),
		Phase:     fleetv1alpha1.ProvisionPhaseRunning,
		ActorKind: "Controller",
		ActorName: "cluster-pipeline",
		JobName:   jobName,
		Message:   message,
		StartedAt: &now,
	}
	return siteprovision.CommitAction(ctx, r.Client, site, rec, false)
}

// ensurePipelineHosts creates any Declared VM guests that back the cluster nodes,
// one connector action at a time. Once every node host is Ready it lets the normal
// Pending handler issue the bootstrap bundle.
func (r *ClusterJoinReconciler) ensurePipelineHosts(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin) (ctrl.Result, bool, error) {
	ready, res, err := r.pipelineHostsReady(ctx, fcj)
	if err != nil {
		return ctrl.Result{}, true, err
	}
	if !ready {
		return res, true, nil
	}
	if fcj.Status.PipelineStep != "bootstrap" {
		fcj.Status.PipelineStep = "bootstrap"
		fcj.Status.Message = "Hosts ready; awaiting bootstrap bundle"
		if err := r.Status().Update(ctx, fcj); err != nil {
			return ctrl.Result{}, true, err
		}
	}
	return ctrl.Result{Requeue: true}, false, nil
}

// pipelineHostsReady reports whether every cluster node has a Ready backing host,
// starting create_vm for Declared VM guests as it goes.
func (r *ClusterJoinReconciler) pipelineHostsReady(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin) (bool, ctrl.Result, error) {
	site, err := sites.FindFleetSite(ctx, r.Client, r.Cfg.Namespace, fcj.Spec.SiteId)
	if err != nil {
		return false, ctrl.Result{}, fmt.Errorf("site %q not found for autoProvision cluster", fcj.Spec.SiteId)
	}
	if strings.TrimSpace(site.Spec.EffectiveProvisionProvider()) != fleetv1alpha1.ProvisionProviderYaidap {
		return true, ctrl.Result{}, nil
	}
	waitAfter := time.Duration(r.Cfg.RequeueIntervalSec) * time.Second
	if waitAfter <= 0 {
		waitAfter = 15 * time.Second
	}
	for _, node := range fcj.Spec.Nodes {
		host := strings.TrimSpace(node.Hostname)
		if host == "" {
			continue
		}
		view, found, err := yaidap.LookupInventoryHost(ctx, r.Client, r.Cfg.Namespace, site.Name, host)
		if err != nil {
			return false, ctrl.Result{}, err
		}
		if !found {
			// External/BM nodes are not inventory guests; nothing to create.
			continue
		}
		if view.Kind != "" && view.Kind != fleetv1alpha1.SiteHostKindVM {
			continue
		}
		switch view.Status.Phase {
		case fleetv1alpha1.SiteHostPhaseReady:
			continue
		case fleetv1alpha1.SiteHostPhaseProvisioning, fleetv1alpha1.SiteHostPhaseDestroying:
			return false, ctrl.Result{RequeueAfter: waitAfter}, nil
		}
		if strings.TrimSpace(view.AnsibleHost) == "" {
			return false, ctrl.Result{}, fmt.Errorf("host %q has no ansible_host; set one before autoProvision", host)
		}
		if err := r.startPipelineCreateVM(ctx, site, host); err != nil {
			return false, ctrl.Result{}, err
		}
		fcj.Status.PipelineStep = "create_vm:" + host
		fcj.Status.Message = "Creating VM " + host
		if err := r.Status().Update(ctx, fcj); err != nil {
			return false, ctrl.Result{}, err
		}
		return false, ctrl.Result{RequeueAfter: waitAfter}, nil
	}
	return true, ctrl.Result{}, nil
}

// startPipelineCreateVM runs create_vm for a guest through the shared site
// provision action (a connector pod) and records it on the FleetSite.
func (r *ClusterJoinReconciler) startPipelineCreateVM(ctx context.Context, site *fleetv1alpha1.FleetSite, host string) error {
	now := metav1.NewTime(time.Now().UTC())
	rec := fleetv1alpha1.SiteProvisionActionRecord{
		ID:        pipelineActionID(),
		Action:    fleetv1alpha1.ProvisionActionCreateVM,
		Subject:   host,
		Provider:  fleetv1alpha1.ProvisionProviderYaidap,
		Phase:     fleetv1alpha1.ProvisionPhasePending,
		ActorKind: "Controller",
		ActorName: "cluster-pipeline",
		StartedAt: &now,
	}
	jobName, err := siteprovision.StartYaidapAction(ctx, r.Client, r.Cfg, site, &rec, siteprovision.StartYaidapActionOptions{
		GuestFound: true,
		ActorKind:  "Controller",
		ActorName:  "cluster-pipeline",
	})
	if err != nil {
		return err
	}
	rec.Phase = fleetv1alpha1.ProvisionPhaseRunning
	rec.Message = "YAIDAP connector job started"
	rec.JobName = jobName
	return siteprovision.CommitAction(ctx, r.Client, site, rec, false)
}

// ensurePipelineBootstrap starts the bootstrap Job and waits for it; the node
// reports progress so the phase advances to Validated.
func (r *ClusterJoinReconciler) ensurePipelineBootstrap(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, bool, error) {
	if fcj.Spec.NetworkMode != "" && fcj.Spec.NetworkMode != fleetv1alpha1.NetworkModeWireguard {
		return ctrl.Result{}, true, fmt.Errorf("autoProvision requires wireguard network mode")
	}
	if fcj.Status.BootstrapJobName == "" {
		job, err := clusterbootstrap.JobForCluster(ctx, r.Client, r.Cfg, fcj)
		if err != nil {
			// Bundle not issued yet; wait rather than fail.
			fcj.Status.Message = "Waiting for bootstrap bundle: " + err.Error()
			_ = r.Status().Update(ctx, fcj)
			return ctrl.Result{RequeueAfter: 15 * time.Second}, true, nil
		}
		if err := clusterbootstrap.EnsureJob(ctx, r.Client, job); err != nil {
			return ctrl.Result{}, true, err
		}
		fcj.Status.BootstrapJobName = job.Name
		fcj.Status.PipelineStep = "bootstrap"
		fcj.Status.Message = "Running bootstrap via jumphost"
		if err := r.Status().Update(ctx, fcj); err != nil {
			return ctrl.Result{}, true, err
		}
		return ctrl.Result{Requeue: true}, true, nil
	}
	jobName := fcj.Status.BootstrapJobName
	// Record the step in the site provision history (a fresh action per run, tracked
	// by status.bootstrapActionID) so a re-run shows a new operation, not the previous
	// run's, with logs next to the create_vm entries.
	if fcj.Status.BootstrapActionID == "" {
		id := pipelineActionID()
		if err := r.commitPipelineAction(ctx, fcj, fleetv1alpha1.ProvisionActionBootstrap, jobName, id, "Bootstrap via jumphost"); err != nil {
			return ctrl.Result{}, true, err
		}
		fcj.Status.BootstrapActionID = id
		if err := r.Status().Update(ctx, fcj); err != nil {
			return ctrl.Result{}, true, err
		}
	}
	done, ok, msg := r.jobDone(ctx, fcj.Namespace, jobName)
	if !done {
		return ctrl.Result{RequeueAfter: 20 * time.Second}, true, nil
	}
	if !ok {
		r.finalizePipelineAction(ctx, fcj, fcj.Status.BootstrapActionID, fleetv1alpha1.ProvisionActionBootstrap, jobName, fleetv1alpha1.ProvisionPhaseFailed, "Bootstrap failed: "+msg)
		fcj.Status.Phase = fleetv1alpha1.JoinPhaseFailed
		fcj.Status.BootstrapJobName = ""
		fcj.Status.PipelineStep = "bootstrap-failed"
		fcj.Status.Message = "Bootstrap failed: " + msg
		if err := r.Status().Update(ctx, fcj); err != nil {
			return ctrl.Result{}, true, err
		}
		return ctrl.Result{}, true, nil
	}
	// Finalize the history entry here: the pipeline replaces the site's current
	// action with the provision step, so the FleetSite reconciler would otherwise
	// leave this one showing Running.
	r.finalizePipelineAction(ctx, fcj, fcj.Status.BootstrapActionID, fleetv1alpha1.ProvisionActionBootstrap, jobName, fleetv1alpha1.ProvisionPhaseSucceeded, "Bootstrap complete")
	fcj.Status.BootstrapJobName = ""
	fcj.Status.PipelineStep = "provision"
	fcj.Status.Message = "Bootstrap complete; awaiting validation"
	if err := r.Status().Update(ctx, fcj); err != nil {
		return ctrl.Result{}, true, err
	}
	return ctrl.Result{Requeue: true}, true, nil
}

// finalizePipelineAction marks a cluster bring-up history entry finished with the
// captured job log tail.
func (r *ClusterJoinReconciler) finalizePipelineAction(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, id, action, jobName, phase, message string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	site, err := sites.FindFleetSite(ctx, r.Client, r.Cfg.Namespace, fcj.Spec.SiteId)
	if err != nil {
		return
	}
	tail := ""
	if r.Clientset != nil && strings.TrimSpace(jobName) != "" {
		if logs, err := ansiblelogs.PodLogs(ctx, r.Clientset, fcj.Namespace, jobName); err == nil {
			tail = ansiblelogs.Tail(logs, 65536)
		}
	}
	now := metav1.NewTime(time.Now().UTC())
	rec := fleetv1alpha1.SiteProvisionActionRecord{
		ID:        id,
		Action:    action,
		Subject:   fcj.Spec.EffectiveClusterName(fcj.Name),
		Provider:  site.Spec.EffectiveProvisionProvider(),
		Phase:     phase,
		ActorKind: "Controller",
		ActorName: "cluster-pipeline",
		JobName:   jobName,
		Message:   message,
		LogTail:   tail,
		StartedAt: &now,
		EndedAt:   &now,
	}
	_ = siteprovision.CommitAction(ctx, r.Client, site, rec, true)
}

func pipelineActionID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
