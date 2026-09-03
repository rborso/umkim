package reconcile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/overlay"
	"github.com/rborso/umkim/controllers/fleet-site/internal/script"
	"github.com/rborso/umkim/controllers/fleet-site/internal/wg"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	phaseDraft        = "Draft"
	phaseScriptIssued = "ScriptIssued"
	phaseEnrolled     = "Enrolled"
	phaseValidated    = "Validated"
	phaseProvisioning = "Provisioning"
	phaseReady        = "Ready"
	phaseFailed       = "Failed"
)

type Reconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Cfg       config.Config
	Clientset kubernetes.Interface
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var fs fleetv1alpha1.FleetSite
	if err := r.Get(ctx, req.NamespacedName, &fs); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.reconcileEdgeDomains(ctx); err != nil {
		logger.Error(err, "reconcile edge domains")
	}

	if fs.Spec.IsRegistry() {
		if fs.Status.Phase == "" || fs.Status.Phase == phaseDraft {
			fs.Status.Phase = phaseReady
			fs.Status.Message = "Registry site (datacenter metadata)"
			_ = r.Status().Update(ctx, &fs)
		}
		return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
	}

	if fs.Status.Phase == phaseReady {
		return ctrl.Result{}, nil
	}

	siteName := fs.Spec.SiteName
	if siteName == "" {
		siteName = fs.Name
	}

	if fs.Status.Phase == "" {
		fs.Status.Phase = phaseDraft
	}

	// Auto-issue script from Draft
	if fs.Status.Phase == phaseDraft {
		if err := r.issueScript(ctx, &fs, siteName); err != nil {
			return r.fail(ctx, &fs, err)
		}
		return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
	}

	// Handle annotation triggers
	if fs.Annotations != nil {
		if _, ok := fs.Annotations["fleet.umkim.com/validate"]; ok && fs.Status.Phase == phaseEnrolled {
			delete(fs.Annotations, "fleet.umkim.com/validate")
			_ = r.Update(ctx, &fs)
			if err := r.startValidate(ctx, &fs, siteName); err != nil {
				return r.fail(ctx, &fs, err)
			}
			return ctrl.Result{Requeue: true}, nil
		}
		if _, ok := fs.Annotations["fleet.umkim.com/provision"]; ok && fs.Status.Phase == phaseValidated {
			delete(fs.Annotations, "fleet.umkim.com/provision")
			_ = r.Update(ctx, &fs)
			if err := r.startProvision(ctx, &fs, siteName); err != nil {
				return r.fail(ctx, &fs, err)
			}
			return ctrl.Result{Requeue: true}, nil
		}
		if _, ok := fs.Annotations["fleet.umkim.com/issue-script"]; ok {
			delete(fs.Annotations, "fleet.umkim.com/issue-script")
			_ = r.Update(ctx, &fs)
			fs.Status.Phase = phaseDraft
			if err := r.issueScript(ctx, &fs, siteName); err != nil {
				return r.fail(ctx, &fs, err)
			}
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Watch validate job (phase stays Enrolled until complete)
	if fs.Status.ValidateJobName != "" && fs.Status.Phase == phaseEnrolled {
		done, ok, msg := r.jobDone(ctx, fs.Namespace, fs.Status.ValidateJobName)
		if !done {
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		fs.Status.ValidateJobName = ""
		if ok {
			fs.Status.Phase = phaseValidated
			fs.Status.Message = "Validation passed"
			fs.Status.ValidationErrors = nil
		} else {
			fs.Status.Phase = phaseFailed
			fs.Status.Message = msg
		}
		return ctrl.Result{}, r.Status().Update(ctx, &fs)
	}

	// Watch provision job
	if fs.Status.Phase == phaseProvisioning && fs.Status.ProvisionJobName != "" {
		done, ok, msg := r.jobDone(ctx, fs.Namespace, fs.Status.ProvisionJobName)
		if !done {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		fs.Status.ProvisionJobName = ""
		if ok {
			avail, _ := r.managedClusterAvailable(ctx, siteName)
			fs.Status.ManagedClusterAvailable = avail
			if avail {
				fs.Status.Phase = phaseReady
				fs.Status.Message = "Site provisioned and joined to UMKIM"
			} else {
				fs.Status.Phase = phaseFailed
				fs.Status.Message = "Provision job finished but ManagedCluster not Available"
			}
		} else {
			fs.Status.Phase = phaseFailed
			fs.Status.Message = msg
		}
		return ctrl.Result{}, r.Status().Update(ctx, &fs)
	}

	// Check Ready: MC available while Enrolled+ (skip if already provisioning)
	if fs.Status.Phase == phaseEnrolled || fs.Status.Phase == phaseValidated {
		_ = r.ensureSSHSecret(ctx, &fs, siteName)
	}

	logger.Info("reconcile idle", "site", siteName, "phase", fs.Status.Phase)
	return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
}

func (r *Reconciler) issueScript(ctx context.Context, fs *fleetv1alpha1.FleetSite, siteName string) error {
	hubPub := r.hubPublicKey(ctx)
	if hubPub == "" {
		return fmt.Errorf("hub wireguard public key not configured")
	}

	used, err := r.usedOverlayIPs(ctx)
	if err != nil {
		return err
	}
	addr := fs.Status.WireguardAddress
	if addr == "" {
		addr, err = wg.AllocateAddress(used, r.Cfg.PoolStart, r.Cfg.PoolEnd)
		if err != nil {
			return err
		}
	}

	priv, pub, enrollToken := "", "", ""
	var prior corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: bootstrapSecretName(siteName), Namespace: fs.Namespace}, &prior); err == nil {
		priv = strings.TrimSpace(string(prior.Data["wg-private-key"]))
		pub = strings.TrimSpace(string(prior.Data["wg-public-key"]))
		enrollToken = strings.TrimSpace(string(prior.Data["enrollment-token"]))
	}
	if priv == "" || pub == "" {
		var err error
		priv, pub, err = wg.GenKeyPair()
		if err != nil {
			return err
		}
	}
	if enrollToken == "" {
		tok := make([]byte, 32)
		if _, err := rand.Read(tok); err != nil {
			return err
		}
		enrollToken = hex.EncodeToString(tok)
	}

	wgBlock := script.WGConfigBlock(priv, pub, addr, hubPub, r.Cfg.WireguardEndpointHost, r.Cfg.WireguardEndpointPort, r.Cfg.WireguardKeepalive, r.Cfg.WireguardSubnet)
	bootstrap, err := script.Render(script.Params{
		SiteName:          siteName,
		EnrollmentToken:   enrollToken,
		HubWireguardIP:    r.Cfg.HubWireguardIP,
		EnrollmentBaseURL: r.Cfg.EnrollmentBaseURL,
		WGConfigBlock:     wgBlock,
		WGPublicKey:       pub,
		RockyMajor:        r.Cfg.RockyMajor(fs.Spec.RockyVersion),
	})
	if err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bootstrapSecretName(siteName),
			Namespace: fs.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"bootstrap.sh":    bootstrap,
			"enrollment-token": enrollToken,
			"wg-private-key":  priv,
			"wg-public-key":   pub,
		},
	}
	if err := controllerutil.SetControllerReference(fs, secret, r.Scheme); err != nil {
		return err
	}
	var existing corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, secret); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		existing.StringData = secret.StringData
		if err := r.Update(ctx, &existing); err != nil {
			return err
		}
	}

	// WG sync job on hub
	jobName := fmt.Sprintf("fleet-site-wg-%s-%d", siteName, time.Now().Unix())
	removePeers := []jobs.WGPeer{}
	oldPub := strings.TrimSpace(string(prior.Data["wg-public-key"]))
	if oldPub == "" {
		oldPub = strings.TrimSpace(fs.Status.WireguardPublicKey)
	}
	if oldPub != "" && oldPub != pub {
		removePeers = append(removePeers, jobs.WGPeer{PublicKey: oldPub})
	}
	wgJob := jobs.WgSyncJob(fs.Namespace, jobName, pub, wg.HostIP(addr), removePeers)
	if err := controllerutil.SetControllerReference(fs, wgJob, r.Scheme); err != nil {
		return err
	}
	_ = r.Create(ctx, wgJob)

	if err := r.ensureInventoryCM(ctx, fs, siteName, addr); err != nil {
		return err
	}
	if err := r.ensureSSHSecret(ctx, fs, siteName); err != nil {
		return err
	}

	fs.Status.Phase = phaseScriptIssued
	fs.Status.Message = "Bootstrap script ready for download"
	fs.Status.WireguardAddress = addr
	fs.Status.WireguardPublicKey = pub
	return r.Status().Update(ctx, fs)
}

func (r *Reconciler) startValidate(ctx context.Context, fs *fleetv1alpha1.FleetSite, siteName string) error {
	if err := r.ensureInventoryCM(ctx, fs, siteName, fs.Status.WireguardAddress); err != nil {
		return err
	}
	if err := r.ensureSSHSecret(ctx, fs, siteName); err != nil {
		return err
	}
	jobName := "fleet-site-validate-" + siteName
	siteID := fs.Spec.EffectiveSiteID(fs.Name)
	j := jobs.ValidateJob(fs.Namespace, siteName, siteName, siteID, fleetv1alpha1.ClusterTopologySingleNode, r.Cfg.AnsibleRunnerImage, sshSecretName(siteName), inventoryCMName(siteName))
	j.Name = jobName
	if err := controllerutil.SetControllerReference(fs, j, r.Scheme); err != nil {
		return err
	}
	_ = r.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: fs.Namespace}}, client.PropagationPolicy(metav1.DeletePropagationBackground))
	if err := r.Create(ctx, j); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	fs.Status.ValidateJobName = jobName
	fs.Status.Message = "Validation running"
	return r.Status().Update(ctx, fs)
}

func (r *Reconciler) startProvision(ctx context.Context, fs *fleetv1alpha1.FleetSite, siteName string) error {
	jobName := "fleet-site-provision-" + siteName
	siteID := fs.Spec.EffectiveSiteID(fs.Name)
	j := jobs.ProvisionJob(fs.Namespace, siteName, siteName, siteID, fleetv1alpha1.ClusterTopologySingleNode, false, r.Cfg.AnsibleRunnerImage, sshSecretName(siteName), inventoryCMName(siteName))
	j.Name = jobName
	if err := controllerutil.SetControllerReference(fs, j, r.Scheme); err != nil {
		return err
	}
	_ = r.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: fs.Namespace}}, client.PropagationPolicy(metav1.DeletePropagationBackground))
	if err := r.Create(ctx, j); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	fs.Status.Phase = phaseProvisioning
	fs.Status.ProvisionJobName = jobName
	fs.Status.Message = "Provisioning site (Ansible)"
	return r.Status().Update(ctx, fs)
}

func (r *Reconciler) jobDone(ctx context.Context, ns, name string) (done, ok bool, msg string) {
	return r.jobDoneAllowMissing(ctx, ns, name, false)
}

// jobPollState reports Job completion. A missing Job is terminal (failed) unless allowMissing is true.
func (r *Reconciler) jobPollState(ctx context.Context, ns, name string, allowMissing bool) (done, ok bool, msg string) {
	if name == "" {
		return true, false, "job name not set"
	}
	var job batchv1.Job
	if err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			if allowMissing {
				return true, true, ""
			}
			return true, false, "job not found (expired or removed)"
		}
		return false, false, err.Error()
	}
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true, true, ""
		}
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			msg := c.Message
			if msg == "" {
				msg = "job failed"
			}
			return true, false, msg
		}
	}
	return false, false, ""
}

func (r *Reconciler) jobActive(ctx context.Context, ns, name string) bool {
	if name == "" {
		return false
	}
	done, _, _ := r.jobPollState(ctx, ns, name, false)
	return !done
}

// jobIsFinished reports whether a Job has reached a terminal condition.
func jobIsFinished(job *batchv1.Job) (done, ok bool) {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true, true
		}
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true, false
		}
	}
	return false, false
}

// jobDoneAllowMissing treats a missing Job as complete when allowMissing is true (delete teardown jobs may be GC'd).
func (r *Reconciler) jobDoneAllowMissing(ctx context.Context, ns, name string, allowMissing bool) (done, ok bool, msg string) {
	var job batchv1.Job
	if err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			if allowMissing {
				return true, true, ""
			}
			return false, false, ""
		}
		return false, false, err.Error()
	}
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true, true, ""
		}
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true, false, c.Message
		}
	}
	return false, false, ""
}

func (r *Reconciler) managedClusterAvailable(ctx context.Context, name string) (bool, error) {
	var mc unstructured.Unstructured
	mc.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	if err := r.Get(ctx, types.NamespacedName{Name: name}, &mc); err != nil {
		return false, err
	}
	conds, _, _ := unstructured.NestedSlice(mc.Object, "status", "conditions")
	for _, c := range conds {
		m, _ := c.(map[string]interface{})
		if m["type"] == "ManagedClusterConditionAvailable" && m["status"] == "True" {
			return true, nil
		}
	}
	return false, nil
}

func (r *Reconciler) usedOverlayIPs(ctx context.Context) (map[string]bool, error) {
	return overlay.CollectUsed(ctx, r.Client, r.Cfg.Namespace, r.Cfg.ReservedOverlayHosts)
}

func (r *Reconciler) hubPublicKey(ctx context.Context) string {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Cfg.Namespace, Name: "fleet-site-hub-wg-pubkey"}, &secret); err == nil {
		if k := strings.TrimSpace(string(secret.Data["publickey"])); k != "" {
			return k
		}
	}
	return strings.TrimSpace(r.Cfg.HubWireguardPublicKey)
}

func (r *Reconciler) ensureInventoryCM(ctx context.Context, fs *fleetv1alpha1.FleetSite, siteName, addr string) error {
	host := wg.HostIP(addr)
	enableKubevirt := fs.Spec.GPU || (fs.Spec.Storage != nil && fs.Spec.Storage.MinDataDiskGi > 0)
	kubevirtEnable := "false"
	if enableKubevirt {
		kubevirtEnable = "true"
	}
	kubevirtOlmAuto := "false"
	ini := fmt.Sprintf(`[site_bootstrap]
site-%s ansible_host=%s ansible_user=%s

[site_bootstrap:vars]
ocm_cluster_name=%s
fleet_site_name=%s
fleet_site_wg_cidr=%s
fleet_site_wg_ip=%s
wireguard_address=%s
ansible_ssh_common_args=-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null

[%[1]s]
%[1]s ansible_host=%s

[%[1]s:vars]
fleet_site_enable_kubevirt=%s
fleet_site_auto_install_kubevirt_operator=%s

[ocm_spoke_join:children]
site_bootstrap

[hub]
%s ansible_connection=local

[hub:vars]
wireguard_address=10.10.0.1/24
ocm_kubeconfig=/etc/rancher/k3s/k3s.yaml
k3s_command_path=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
`, siteName, host, r.Cfg.SSHUser, siteName, siteName, addr, host, addr, siteName, host, kubevirtEnable, kubevirtOlmAuto, r.Cfg.HubHostInventoryName)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: inventoryCMName(siteName), Namespace: fs.Namespace},
		Data:       map[string]string{"inventory.ini": ini},
	}
	if err := controllerutil.SetControllerReference(fs, cm, r.Scheme); err != nil {
		return err
	}
	var ex corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, &ex); apierrors.IsNotFound(err) {
		return r.Create(ctx, cm)
	}
	ex.Data = cm.Data
	return r.Update(ctx, &ex)
}

func (r *Reconciler) ensureSSHSecret(ctx context.Context, fs *fleetv1alpha1.FleetSite, siteName string) error {
	name := sshSecretName(siteName)
	var ex corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: fs.Namespace}, &ex); err == nil {
		if len(ex.Data["id_rsa"]) > 0 {
			return nil
		}
	}
	// Copy from cluster-wide bootstrap key secret if present (Ansible fleet_site_controller role).
	const shared = "fleet-site-ansible-ssh"
	var sharedSec corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: shared, Namespace: fs.Namespace}, &sharedSec); err == nil && len(sharedSec.Data["id_rsa"]) > 0 {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: fs.Namespace},
			Type:       corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"id_rsa": sharedSec.Data["id_rsa"],
			},
		}
		if err := controllerutil.SetControllerReference(fs, secret, r.Scheme); err != nil {
			return err
		}
		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: fs.Namespace}, &ex); apierrors.IsNotFound(err) {
			return r.Create(ctx, secret)
		}
		ex.Data = secret.Data
		return r.Update(ctx, &ex)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: fs.Namespace},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"README": "Populate id_rsa or create Secret fleet-site-ansible-ssh with the bootstrap SSH key",
		},
	}
	_ = controllerutil.SetControllerReference(fs, secret, r.Scheme)
	return client.IgnoreAlreadyExists(r.Create(ctx, secret))
}

func (r *Reconciler) fail(ctx context.Context, fs *fleetv1alpha1.FleetSite, err error) (ctrl.Result, error) {
	fs.Status.Phase = phaseFailed
	fs.Status.Message = err.Error()
	_ = r.Status().Update(ctx, fs)
	return ctrl.Result{}, err
}

func bootstrapSecretName(siteName string) string  { return "fleet-site-" + siteName + "-bootstrap" }
func sshSecretName(siteName string) string         { return "fleet-site-" + siteName + "-ansible-ssh" }
func inventoryCMName(siteName string) string       { return "fleet-site-" + siteName + "-inventory" }

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&fleetv1alpha1.FleetSite{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Secret{}).
		Complete(r); err != nil {
		return err
	}
	cjr := &ClusterJoinReconciler{Reconciler: r}
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&fleetv1alpha1.FleetClusterJoin{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Complete(cjr); err != nil {
		return err
	}
	if err := r.SetupIngressAuthWithManager(mgr); err != nil {
		return err
	}
	if err := r.SetupAcmeWithManager(mgr); err != nil {
		return err
	}
	if err := r.SetupLLMWithManager(mgr); err != nil {
		return err
	}
	if err := r.SetupAppWithManager(mgr); err != nil {
		return err
	}
	if err := r.SetupChartWithManager(mgr); err != nil {
		return err
	}
	return nil
}
