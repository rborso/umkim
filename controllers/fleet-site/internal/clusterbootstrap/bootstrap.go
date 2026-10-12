// Package clusterbootstrap builds and starts the hub Job that runs a cluster's
// bootstrap bundle on its first control node via the site jumphost.
package clusterbootstrap

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sites"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// JobName returns the deterministic bootstrap Job name for a cluster.
func JobName(clusterName string) string {
	return "fleet-cluster-bootstrap-" + clusterName
}

// BundleSecretName is the Secret holding the generated bootstrap bundle.
func BundleSecretName(clusterName string) string {
	return "fleet-cluster-" + clusterName + "-bootstrap"
}

// JobForCluster builds (but does not create) the bootstrap Job. It resolves the
// first control node, jumphost, and site provision SSH secret from the cluster and
// site; a missing prerequisite is returned as an error.
func JobForCluster(ctx context.Context, c client.Client, cfg config.Config, fcj *fleetv1alpha1.FleetClusterJoin) (*batchv1.Job, error) {
	name := fcj.Spec.EffectiveClusterName(fcj.Name)
	if fcj.Spec.NetworkMode != "" && fcj.Spec.NetworkMode != fleetv1alpha1.NetworkModeWireguard {
		return nil, fmt.Errorf("bootstrap automation requires wireguard network mode")
	}
	target := FirstControlAddress(fcj)
	if target == "" {
		return nil, fmt.Errorf("cluster has no control node address")
	}
	var site *fleetv1alpha1.FleetSite
	if fs, err := sites.FindFleetSite(ctx, c, cfg.Namespace, fcj.Spec.SiteId); err == nil {
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
		return nil, fmt.Errorf("no jumphost configured for this cluster or site")
	}
	// The bootstrap runs from the jumphost, so it needs the key the jumphost
	// authorizes (the site provision secret), not the cluster Ansible key.
	sshSecret := ""
	if site != nil && site.Spec.Provision != nil {
		sshSecret = strings.TrimSpace(site.Spec.Provision.SSHSecretRef)
	}
	if sshSecret == "" {
		return nil, fmt.Errorf("site provision SSH secret required to bootstrap via jumphost")
	}
	bundleSecret := BundleSecretName(name)
	var sec corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: cfg.Namespace, Name: bundleSecret}, &sec); err != nil {
		return nil, fmt.Errorf("bootstrap bundle not issued: %w", err)
	}
	if len(sec.Data["bootstrap.tar.gz"]) == 0 {
		return nil, fmt.Errorf("bootstrap bundle not available")
	}
	jumpUser, jumpHost, jumpPort := SplitJumphost(jumphost, cfg.SSHUser)
	return jobs.ClusterBootstrapJob(jobs.ClusterBootstrapJobOpts{
		Namespace:    cfg.Namespace,
		JobName:      JobName(name),
		Cluster:      name,
		TargetHost:   target,
		SSHUser:      cfg.SSHUser,
		JumpUser:     jumpUser,
		JumpHost:     jumpHost,
		JumpPort:     jumpPort,
		RunnerImage:  cfg.AnsibleRunnerImage,
		BundleSecret: bundleSecret,
		SSHSecret:    sshSecret,
	}), nil
}

// EnsureJob starts the bootstrap Job, replacing a previously finished one so a
// retry reruns the pipeline. A still-running Job is left alone.
func EnsureJob(ctx context.Context, c client.Client, job *batchv1.Job) error {
	key := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
	var existing batchv1.Job
	if err := c.Get(ctx, key, &existing); err == nil {
		if !Finished(&existing) {
			return nil
		}
		_ = c.Delete(ctx, &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: job.Name, Namespace: job.Namespace},
		}, client.PropagationPolicy(metav1.DeletePropagationForeground))
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	if err := c.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// Finished reports whether a Job has completed or failed.
func Finished(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// FirstControlAddress returns the first control node address (or the bootstrap host).
func FirstControlAddress(fcj *fleetv1alpha1.FleetClusterJoin) string {
	for _, n := range fcj.Spec.Nodes {
		if n.EffectiveRole() == fleetv1alpha1.NodeRoleControl && strings.TrimSpace(n.Address) != "" {
			return strings.TrimSpace(n.Address)
		}
	}
	return strings.TrimSpace(fcj.Spec.BootstrapHost)
}

// SplitJumphost parses user@host:port, filling defaults.
func SplitJumphost(raw, defaultUser string) (user, host, port string) {
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
