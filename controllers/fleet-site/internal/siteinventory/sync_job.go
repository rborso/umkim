package siteinventory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
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

const (
	JobPrefix           = "fleet-site-inv-"
	provisionInvCMFmt   = "fleet-site-%s-provision-inventory"
)

// InventorySyncInProgress reports whether a sync job is running or meta marks syncing.
func InventorySyncInProgress(ctx context.Context, c client.Client, ns, siteName string) (bool, error) {
	meta, err := connectors.LoadMeta(ctx, c, ns, siteName, fleetv1alpha1.ProvisionProviderYaidap)
	if err == nil && meta.SyncPhase == "syncing" {
		return true, nil
	}
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

// EnqueueInventorySync pushes hub inventory to the jumphost when no sync is already running.
func EnqueueInventorySync(ctx context.Context, c client.Client, cfg config.Config, fs *fleetv1alpha1.FleetSite) (string, error) {
	if fs.Spec.Provision == nil {
		return "", fmt.Errorf("site has no provision spec")
	}
	busy, err := InventorySyncInProgress(ctx, c, cfg.Namespace, fs.Name)
	if err != nil {
		return "", err
	}
	if busy {
		return "", nil
	}
	spec := fs.Spec.Provision
	bundle, err := yaidap.ProjectFromStore(ctx, c, cfg.Namespace, fs.Name)
	if err != nil {
		return "", err
	}
	syncExtra, err := yaidap.SyncExtraVars(bundle)
	if err != nil {
		return "", err
	}
	cmName, err := EnsureJumphostInventoryCM(ctx, c, cfg, fs)
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
	jobName, err := CreateInventoryConnectorJob(ctx, c, cfg, fs, extra, cmName)
	if err != nil {
		return "", err
	}
	if err := yaidap.MarkSyncing(ctx, c, cfg.Namespace, fs.Name); err != nil {
		return "", err
	}
	return jobName, nil
}

// EnsureJumphostInventoryCM creates or updates the jumphost ansible inventory ConfigMap.
func EnsureJumphostInventoryCM(ctx context.Context, c client.Client, cfg config.Config, fs *fleetv1alpha1.FleetSite) (string, error) {
	spec := fs.Spec.Provision
	cmName := fmt.Sprintf(provisionInvCMFmt, fs.Name)
	ini := JumphostInventoryINI(spec.Jumphost, cfg.SSHUser)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cfg.Namespace,
			Labels:    map[string]string{"app": "fleet-site-provision", "fleet-site": fs.Name},
		},
		Data: map[string]string{"inventory.ini": ini},
	}
	var existing corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: cfg.Namespace, Name: cmName}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			if err := c.Create(ctx, cm); err != nil {
				return "", err
			}
			return cmName, nil
		}
		return "", err
	}
	existing.Data = cm.Data
	if err := c.Update(ctx, &existing); err != nil {
		return "", err
	}
	return cmName, nil
}

// CreateInventoryConnectorJob starts site-provision-connector.yml for inventory operations.
func CreateInventoryConnectorJob(ctx context.Context, c client.Client, cfg config.Config, fs *fleetv1alpha1.FleetSite, extra []string, cmName string) (string, error) {
	spec := fs.Spec.Provision
	jobName := JobPrefix + newJobID()
	var old batchv1.Job
	if err := c.Get(ctx, types.NamespacedName{Namespace: cfg.Namespace, Name: jobName}, &old); err == nil {
		_ = c.Delete(ctx, &old, client.PropagationPolicy(metav1.DeletePropagationBackground))
	}
	j := jobs.SiteProvisionConnectorJob(
		cfg.Namespace, jobName, fs.Name, fs.Spec.EffectiveSiteID(fs.Name),
		cfg.AnsibleRunnerImage, spec.SSHSecretRef, cmName, extra,
	)
	if err := c.Create(ctx, j); err != nil {
		return "", err
	}
	return jobName, nil
}

// JumphostInventoryINI renders a one-host inventory for the provision connector.
func JumphostInventoryINI(jumphost, defaultUser string) string {
	user, host, port := parseJumphost(jumphost, defaultUser)
	var b strings.Builder
	b.WriteString("[jumphost]\n")
	b.WriteString("jumphost ansible_host=")
	b.WriteString(host)
	b.WriteString(" ansible_user=")
	b.WriteString(user)
	if port != "" && port != "22" {
		b.WriteString(" ansible_port=")
		b.WriteString(port)
	}
	b.WriteString("\n")
	return b.String()
}

func parseJumphost(raw, defaultUser string) (user, host, port string) {
	user = defaultUser
	if user == "" {
		user = "linux"
	}
	port = "22"
	raw = strings.TrimSpace(raw)
	if at := strings.LastIndex(raw, "@"); at >= 0 {
		user = raw[:at]
		raw = raw[at+1:]
	}
	if i := strings.LastIndex(raw, ":"); i >= 0 {
		maybePort := raw[i+1:]
		if maybePort != "" && !strings.Contains(maybePort, ":") {
			port = maybePort
			raw = raw[:i]
		}
	}
	host = raw
	return user, host, port
}

func newJobID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
