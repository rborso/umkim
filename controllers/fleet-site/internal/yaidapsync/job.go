package yaidapsync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/siteinventory"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const JobPrefix = "fleet-site-yaidap-"

// SyncInProgress reports whether a YAIDAP repo sync job is still running.
func SyncInProgress(ctx context.Context, c client.Client, ns, siteName string) (bool, error) {
	var list batchv1.JobList
	if err := c.List(ctx, &list, client.InNamespace(ns), client.MatchingLabels{"fleet-site": siteName}); err != nil {
		return false, err
	}
	for i := range list.Items {
		job := &list.Items[i]
		if !strings.HasPrefix(job.Name, JobPrefix) {
			continue
		}
		if job.Status.Succeeded == 0 && job.Status.Failed == 0 {
			return true, nil
		}
	}
	return false, nil
}

// StartRepoSync enqueues a connector job that clones or updates YAIDAP on the jumphost.
func StartRepoSync(ctx context.Context, c client.Client, cfg config.Config, fs *fleetv1alpha1.FleetSite) (string, error) {
	if fs.Spec.Provision == nil {
		return "", fmt.Errorf("site has no provision spec")
	}
	if err := yaidap.ValidateYaidapRepoSync(fs.Spec.Provision); err != nil {
		return "", err
	}
	busy, err := SyncInProgress(ctx, c, cfg.Namespace, fs.Name)
	if err != nil {
		return "", err
	}
	if busy {
		return "", fmt.Errorf("yaidap repo sync already running")
	}
	spec := fs.Spec.Provision
	extra := yaidap.YaidapRepoSyncExtraVars(spec)
	if keyFile := strings.TrimSpace(spec.YaidapGitSecretRef); keyFile != "" {
		secret, err := loadGitSecret(ctx, c, cfg.Namespace, keyFile)
		if err != nil {
			return "", err
		}
		name := yaidap.GitKeyFileName(secret.Data)
		if name == "" {
			return "", fmt.Errorf("git secret %q missing id_ed25519, deploy_key, or id_rsa", keyFile)
		}
		extra = append(extra, "-e", "fleet_yaidap_git_key_file="+name)
	}
	cmName, err := siteinventory.EnsureJumphostInventoryCM(ctx, c, cfg, fs)
	if err != nil {
		return "", err
	}
	id := randomID()
	jobName := JobPrefix + id
	gitSecret := strings.TrimSpace(spec.YaidapGitSecretRef)
	j := jobs.SiteProvisionConnectorJob(cfg.Namespace, jobName, fs.Name, fs.Spec.EffectiveSiteID(fs.Name),
		cfg.AnsibleRunnerImage, spec.SSHSecretRef, cmName, gitSecret, extra)
	var old batchv1.Job
	if err := c.Get(ctx, client.ObjectKeyFromObject(j), &old); err == nil {
		_ = c.Delete(ctx, &old, client.PropagationPolicy(metav1.DeletePropagationBackground))
	} else if !apierrors.IsNotFound(err) {
		return "", err
	}
	if err := c.Create(ctx, j); err != nil {
		return "", err
	}
	return jobName, nil
}

func loadGitSecret(ctx context.Context, c client.Client, ns, name string) (*corev1.Secret, error) {
	var sec corev1.Secret
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &sec); err != nil {
		return nil, fmt.Errorf("git secret %q: %w", name, err)
	}
	return &sec, nil
}

func randomID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
