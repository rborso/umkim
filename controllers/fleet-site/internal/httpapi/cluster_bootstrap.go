package httpapi

import (
	"context"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sites"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// handleBootstrapCluster runs the cluster bootstrap bundle on the first control
// node via the site jumphost. This is the server-side equivalent of
// `umkim cluster bootstrap`, so the console can create and bring a cluster up
// without a separate manual bundle run.
func (s *Server) handleBootstrapCluster(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := s.requireMutatingActor(w, r); !ok {
		return
	}
	fcj, err := s.getFleetClusterJoin(ctx, name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if fcj.Status.Phase == fleetv1alpha1.JoinPhaseReady {
		http.Error(w, "cluster is already provisioned", http.StatusConflict)
		return
	}
	if fcj.Spec.NetworkMode != "" && fcj.Spec.NetworkMode != fleetv1alpha1.NetworkModeWireguard {
		http.Error(w, "bootstrap automation requires wireguard network mode", http.StatusBadRequest)
		return
	}

	target := firstControlAddress(fcj)
	if target == "" {
		http.Error(w, "cluster has no control node address", http.StatusBadRequest)
		return
	}

	var site *fleetv1alpha1.FleetSite
	if fs, err := sites.FindFleetSite(ctx, s.Client, s.Cfg.Namespace, fcj.Spec.SiteId); err == nil {
		site = fs
	}
	jumphost := strings.TrimSpace(fcj.Spec.Jumphost)
	if jumphost == "" && site != nil {
		jumphost = strings.TrimSpace(site.Spec.Jumphost)
	}
	if jumphost == "" && site != nil && site.Spec.Provision != nil {
		jumphost = strings.TrimSpace(site.Spec.Provision.Jumphost)
	}
	if jumphost == "" {
		http.Error(w, "no jumphost configured for this cluster or site", http.StatusBadRequest)
		return
	}
	// The bootstrap runs from the jumphost, so it needs the key the jumphost
	// authorizes (the site provision secret), not the cluster Ansible key.
	sshSecret := ""
	if site != nil && site.Spec.Provision != nil {
		sshSecret = strings.TrimSpace(site.Spec.Provision.SSHSecretRef)
	}
	if sshSecret == "" {
		http.Error(w, "site provision SSH secret required to bootstrap via jumphost", http.StatusBadRequest)
		return
	}

	bundleSecret := "fleet-cluster-" + name + "-bootstrap"
	var sec corev1.Secret
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: bundleSecret}, &sec); err != nil {
		http.Error(w, "bootstrap bundle not issued", http.StatusNotFound)
		return
	}
	if len(sec.Data["bootstrap.tar.gz"]) == 0 {
		http.Error(w, "bootstrap bundle not available", http.StatusNotFound)
		return
	}

	jumpUser, jumpHost, jumpPort := splitJumphost(jumphost, s.Cfg.SSHUser)
	jobName := "fleet-cluster-bootstrap-" + name
	job := jobs.ClusterBootstrapJob(jobs.ClusterBootstrapJobOpts{
		Namespace:    s.Cfg.Namespace,
		JobName:      jobName,
		Cluster:      name,
		TargetHost:   target,
		SSHUser:      s.Cfg.SSHUser,
		JumpUser:     jumpUser,
		JumpHost:     jumpHost,
		JumpPort:     jumpPort,
		RunnerImage:  s.Cfg.AnsibleRunnerImage,
		BundleSecret: bundleSecret,
		SSHSecret:    sshSecret,
	})
	if err := s.ensureClusterBootstrapJob(ctx, job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"action": "bootstrap", "job": jobName})
}

// ensureClusterBootstrapJob replaces a finished bootstrap Job and starts a new one.
func (s *Server) ensureClusterBootstrapJob(ctx context.Context, job *batchv1.Job) error {
	key := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
	var existing batchv1.Job
	if err := s.Client.Get(ctx, key, &existing); err == nil {
		if !jobFinished(&existing) {
			return nil
		}
		_ = s.Client.Delete(ctx, &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: job.Name, Namespace: job.Namespace},
		}, client.PropagationPolicy(metav1.DeletePropagationForeground))
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	if err := s.Client.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func firstControlAddress(fcj *fleetv1alpha1.FleetClusterJoin) string {
	for _, n := range fcj.Spec.Nodes {
		if n.EffectiveRole() == fleetv1alpha1.NodeRoleControl && strings.TrimSpace(n.Address) != "" {
			return strings.TrimSpace(n.Address)
		}
	}
	return strings.TrimSpace(fcj.Spec.BootstrapHost)
}

// splitJumphost parses user@host:port, filling defaults.
func splitJumphost(raw, defaultUser string) (user, host, port string) {
	user = strings.TrimSpace(defaultUser)
	if user == "" {
		user = "linux"
	}
	port = "22"
	raw = strings.TrimSpace(raw)
	if at := strings.LastIndex(raw, "@"); at >= 0 {
		if u := strings.TrimSpace(raw[:at]); u != "" {
			user = u
		}
		raw = raw[at+1:]
	}
	if i := strings.LastIndex(raw, ":"); i >= 0 {
		if p := strings.TrimSpace(raw[i+1:]); p != "" && !strings.Contains(p, ":") {
			port = p
			raw = raw[:i]
		}
	}
	host = strings.TrimSpace(raw)
	return user, host, port
}

func jobFinished(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
