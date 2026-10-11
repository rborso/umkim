package reconcile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/bundle"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingress"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingressauth"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/registry"
	"github.com/rborso/umkim/controllers/fleet-site/internal/script"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sites"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ssh"
	"github.com/rborso/umkim/controllers/fleet-site/internal/storage"
	"github.com/rborso/umkim/controllers/fleet-site/internal/teardown"
	"github.com/rborso/umkim/controllers/fleet-site/internal/wg"
	"gopkg.in/yaml.v3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ClusterJoinReconciler struct {
	*Reconciler
}

func (r *ClusterJoinReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := r.Get(ctx, req.NamespacedName, &fcj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	clusterName := fcj.Spec.EffectiveClusterName(fcj.Name)
	if fcj.Status.Phase == fleetv1alpha1.JoinPhaseDeleting || teardown.HasCleanupFinalizer(&fcj) {
		return r.reconcileClusterDelete(ctx, &fcj, clusterName)
	}
	if fcj.Status.Phase == fleetv1alpha1.JoinPhaseReady {
		if fcj.Annotations != nil {
			if _, ok := fcj.Annotations["fleet.umkim.com/provision"]; ok {
				avail, _ := r.managedClusterAvailable(ctx, clusterName)
				if !avail {
					delete(fcj.Annotations, "fleet.umkim.com/provision")
					if err := r.Update(ctx, &fcj); err != nil {
						return ctrl.Result{}, err
					}
					fcj.Status.Phase = fleetv1alpha1.JoinPhaseValidated
					fcj.Status.ProvisionJobName = ""
					fcj.Status.Message = "Retrying hub provision (ManagedCluster unavailable)"
					if err := r.Status().Update(ctx, &fcj); err != nil {
						return ctrl.Result{}, err
					}
					return ctrl.Result{Requeue: true}, nil
				}
				delete(fcj.Annotations, "fleet.umkim.com/provision")
				if err := r.Update(ctx, &fcj); err != nil {
					return ctrl.Result{}, err
				}
				fcj.Status.Message = "provision ignored; cluster is Ready"
				if err := r.Status().Update(ctx, &fcj); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
		ensureProviderStatusDefaults(&fcj, clusterName)
		// Sync fleet-edge-clusters before any Ready sub-step that may RequeueAfter.
		// Overlay connectivity always returns RequeueAfter while WireGuard is configured,
		// which previously skipped upsert and left exposure_mode/tls_policy stale after
		// console or kubectl patches to FleetClusterJoin.spec.edge.
		if err := r.upsertEdgeClusterEntry(ctx, &fcj, clusterName); err != nil {
			return ctrl.Result{}, err
		}
		if res, err := r.reconcileKubeconfigSecret(ctx, &fcj, clusterName); err != nil {
			return ctrl.Result{}, err
		} else if res.Requeue || res.RequeueAfter > 0 {
			return res, nil
		}
		if res, err := r.reconcileClusterStorage(ctx, &fcj, clusterName); err != nil {
			return ctrl.Result{}, err
		} else if res.Requeue || res.RequeueAfter > 0 {
			return res, nil
		}
		if res, err := r.reconcileClusterKubevirt(ctx, &fcj, clusterName); err != nil {
			return ctrl.Result{}, err
		} else if res.Requeue || res.RequeueAfter > 0 {
			return res, nil
		}
		if res, err := r.reconcileClusterGpu(ctx, &fcj, clusterName); err != nil {
			return ctrl.Result{}, err
		} else if res.Requeue || res.RequeueAfter > 0 {
			return res, nil
		}
		if res, err := r.reconcileCapabilityDiscover(ctx, &fcj, clusterName); err != nil {
			return ctrl.Result{}, err
		} else if res.Requeue || res.RequeueAfter > 0 {
			return res, nil
		}
		if res, err := r.reconcileClusterProviders(ctx, &fcj, clusterName); err != nil {
			return ctrl.Result{}, err
		} else if res.Requeue || res.RequeueAfter > 0 {
			return res, nil
		}
		if res, err := r.reconcileNodeOperation(ctx, &fcj, clusterName); err != nil {
			return ctrl.Result{}, err
		} else if res.Requeue || res.RequeueAfter > 0 {
			return res, nil
		}
		if res, err := r.reconcileOverlayConnectivity(ctx, &fcj, clusterName); err != nil {
			return ctrl.Result{}, err
		} else if res.Requeue || res.RequeueAfter > 0 {
			return res, nil
		}
		avail, err := r.managedClusterAvailable(ctx, clusterName)
		if err == nil && avail != fcj.Status.ManagedClusterAvailable {
			fcj.Status.ManagedClusterAvailable = avail
			_ = r.Status().Update(ctx, &fcj)
		}
		return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
	}

	if fcj.Status.Phase == "" {
		fcj.Status.Phase = fleetv1alpha1.JoinPhasePending
		_ = r.Status().Update(ctx, &fcj)
	}

	// Controller-driven bring-up (spec.autoProvision): create declared VM guests,
	// bootstrap via the jumphost, then provision. Every step runs in a pod.
	if res, handled, err := r.reconcileClusterAutoProvision(ctx, &fcj, clusterName); err != nil {
		return r.failClusterJoin(ctx, &fcj, err)
	} else if handled {
		return res, nil
	}

	if fcj.Status.Phase == fleetv1alpha1.JoinPhasePending {
		if err := r.issueClusterScript(ctx, &fcj, clusterName); err != nil {
			return r.failClusterJoin(ctx, &fcj, err)
		}
		return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
	}

	if r.clusterJoinBundleMode(&fcj) && (fcj.Status.Phase == fleetv1alpha1.JoinPhaseScriptIssued || fcj.Status.Phase == fleetv1alpha1.JoinPhaseEnrolled) {
		if ok, err := r.bootstrapBundleHasSSHKey(ctx, clusterName); err == nil && !ok {
			if err := r.issueClusterScript(ctx, &fcj, clusterName); err != nil {
				return r.failClusterJoin(ctx, &fcj, err)
			}
			return ctrl.Result{Requeue: true}, nil
		}
	}

	if fcj.Annotations != nil {
		if _, ok := fcj.Annotations["fleet.umkim.com/validate"]; ok && fcj.Status.Phase == fleetv1alpha1.JoinPhaseEnrolled {
			delete(fcj.Annotations, "fleet.umkim.com/validate")
			_ = r.Update(ctx, &fcj)
			if r.clusterJoinBundleMode(&fcj) {
				fcj.Status.Message = "Validate runs inside bootstrap bundle; download and run the tar.gz on node 1"
				_ = r.Status().Update(ctx, &fcj)
				return ctrl.Result{Requeue: true}, nil
			}
			if err := r.startClusterValidate(ctx, &fcj, clusterName); err != nil {
				return r.failClusterJoin(ctx, &fcj, err)
			}
			return ctrl.Result{Requeue: true}, nil
		}
		if _, ok := fcj.Annotations["fleet.umkim.com/provision"]; ok {
			if fcj.Status.Phase == fleetv1alpha1.JoinPhaseFailed {
				fcj.Status.Phase = fleetv1alpha1.JoinPhaseValidated
				fcj.Status.Message = "Retrying provision"
				_ = r.Status().Update(ctx, &fcj)
			}
			// Bundle bootstrap enrolls before local validate/progress POST; hub provision
			// must not require Validated when the bundle already ran validate+provision-local.
			if fcj.Status.Phase == fleetv1alpha1.JoinPhaseEnrolled && r.clusterJoinBundleMode(&fcj) {
				fcj.Status.Phase = fleetv1alpha1.JoinPhaseValidated
				fcj.Status.Message = "Bundle enrolled; starting hub provision"
				_ = r.Status().Update(ctx, &fcj)
				return ctrl.Result{Requeue: true}, nil
			}
			if fcj.Status.Phase == fleetv1alpha1.JoinPhaseValidated {
				delete(fcj.Annotations, "fleet.umkim.com/provision")
				_ = r.Update(ctx, &fcj)
				if err := r.startClusterProvision(ctx, &fcj, clusterName); err != nil {
					return r.failClusterJoin(ctx, &fcj, err)
				}
				return ctrl.Result{Requeue: true}, nil
			}
		}
		if _, ok := fcj.Annotations["fleet.umkim.com/issue-script"]; ok {
			delete(fcj.Annotations, "fleet.umkim.com/issue-script")
			_ = r.Update(ctx, &fcj)
			fcj.Status.Phase = fleetv1alpha1.JoinPhasePending
			if err := r.issueClusterScript(ctx, &fcj, clusterName); err != nil {
				return r.failClusterJoin(ctx, &fcj, err)
			}
			return ctrl.Result{Requeue: true}, nil
		}
	}

	if fcj.Status.ValidateJobName != "" && fcj.Status.Phase == fleetv1alpha1.JoinPhaseEnrolled {
		done, ok, msg := r.jobDone(ctx, fcj.Namespace, fcj.Status.ValidateJobName)
		if !done {
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		if ok {
			fcj.Status.Phase = fleetv1alpha1.JoinPhaseValidated
			fcj.Status.Message = "Validation passed"
		} else {
			fcj.Status.Phase = fleetv1alpha1.JoinPhaseFailed
			fcj.Status.Message = "Validation failed: " + msg
		}
		_ = r.Status().Update(ctx, &fcj)
		return ctrl.Result{Requeue: true}, nil
	}

	if fcj.Status.Phase == fleetv1alpha1.JoinPhaseProvisioning && fcj.Status.ProvisionJobName != "" {
		done, ok, msg := r.jobPollState(ctx, fcj.Namespace, fcj.Status.ProvisionJobName, false)
		if !done {
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		if ok {
			if err := r.finishClusterReady(ctx, &fcj, clusterName); err != nil {
				fcj.Status.Phase = fleetv1alpha1.JoinPhaseFailed
				fcj.Status.Message = "Provision finalize failed: " + err.Error()
				_ = r.Status().Update(ctx, &fcj)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			ensureProviderStatusDefaults(&fcj, clusterName)
			fcj.Status.Phase = fleetv1alpha1.JoinPhaseReady
			fcj.Status.Message = "Cluster ready"
			avail, _ := r.managedClusterAvailable(ctx, clusterName)
			fcj.Status.ManagedClusterAvailable = avail
		} else if r.clusterProvisionRecoverable(ctx, clusterName) {
			if err := r.finishClusterReady(ctx, &fcj, clusterName); err != nil {
				fcj.Status.Phase = fleetv1alpha1.JoinPhaseFailed
				fcj.Status.ProvisionJobName = ""
				fcj.Status.Message = "Provision finalize failed: " + err.Error()
			} else {
				ensureProviderStatusDefaults(&fcj, clusterName)
				fcj.Status.Phase = fleetv1alpha1.JoinPhaseReady
				fcj.Status.ProvisionJobName = ""
				fcj.Status.Message = "Cluster ready (recovered after missing provision job)"
				avail, _ := r.managedClusterAvailable(ctx, clusterName)
				fcj.Status.ManagedClusterAvailable = avail
			}
		} else {
			fcj.Status.Phase = fleetv1alpha1.JoinPhaseFailed
			fcj.Status.ProvisionJobName = ""
			fcj.Status.Message = "Provision failed: " + msg
		}
		_ = r.Status().Update(ctx, &fcj)
		return ctrl.Result{}, nil
	}

	// Hub provision job succeeded but finalize (edge registry, labels) failed earlier — retry without re-running Ansible.
	if fcj.Status.Phase == fleetv1alpha1.JoinPhaseFailed && fcj.Status.ProvisionJobName != "" {
		done, ok, _ := r.jobDone(ctx, fcj.Namespace, fcj.Status.ProvisionJobName)
		if done && ok && teardown.ManagedClusterExists(ctx, r.Client, clusterName) {
			if err := r.finishClusterReady(ctx, &fcj, clusterName); err != nil {
				return r.failClusterJoin(ctx, &fcj, err)
			}
			ensureProviderStatusDefaults(&fcj, clusterName)
			fcj.Status.Phase = fleetv1alpha1.JoinPhaseReady
			fcj.Status.Message = "Cluster ready"
			avail, _ := r.managedClusterAvailable(ctx, clusterName)
			fcj.Status.ManagedClusterAvailable = avail
			_ = r.Status().Update(ctx, &fcj)
			return ctrl.Result{}, nil
		}
	}

	// GPU host prep is allowed before bootstrap (ScriptIssued/Enrolled/…); reconcileClusterGpu
	// otherwise runs only on Ready clusters and would leave hostPhase Applying forever.
	if res, err := r.reconcileClusterGpu(ctx, &fcj, clusterName); err != nil {
		return ctrl.Result{}, err
	} else if res.Requeue || res.RequeueAfter > 0 {
		return res, nil
	}

	if fcj.Status.Phase == fleetv1alpha1.JoinPhaseFailed && r.clusterProvisionRecoverable(ctx, clusterName) {
		if provisionFinalizeRetriable(fcj.Status.Message) {
			if err := r.finishClusterReady(ctx, &fcj, clusterName); err != nil {
				fcj.Status.Message = "Provision finalize failed: " + err.Error()
				_ = r.Status().Update(ctx, &fcj)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			ensureProviderStatusDefaults(&fcj, clusterName)
			fcj.Status.Phase = fleetv1alpha1.JoinPhaseReady
			fcj.Status.Message = "Cluster ready"
			avail, _ := r.managedClusterAvailable(ctx, clusterName)
			fcj.Status.ManagedClusterAvailable = avail
			_ = r.Status().Update(ctx, &fcj)
			return ctrl.Result{}, nil
		}
	}

	logger.Info("cluster join idle", "cluster", clusterName, "phase", fcj.Status.Phase)
	return ctrl.Result{RequeueAfter: time.Duration(r.Cfg.RequeueIntervalSec) * time.Second}, nil
}

func (r *ClusterJoinReconciler) issueClusterScript(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) error {
	if teardown.HasCleanupFinalizer(fcj) || fcj.Status.Phase == fleetv1alpha1.JoinPhaseDeleting {
		return nil
	}
	if fcj.Spec.IsLAN() {
		return fmt.Errorf("networkMode lan is deprecated; recreate cluster with wireguard overlay")
	}
	hubPub := r.hubPublicKey(ctx)
	if hubPub == "" {
		return fmt.Errorf("hub wireguard public key not configured")
	}

	secretName := clusterBootstrapSecretName(clusterName)
	var prior corev1.Secret
	enrollToken := ""
	var existingNodes []wg.NodeWG
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: fcj.Namespace}, &prior); err == nil {
		enrollToken = strings.TrimSpace(string(prior.Data["enrollment-token"]))
		existingNodes, _ = wg.NodeWGFromJSON(prior.Data["nodes-wg.json"])
	}
	if enrollToken == "" {
		tok := make([]byte, 32)
		if _, err := rand.Read(tok); err != nil {
			return err
		}
		enrollToken = hex.EncodeToString(tok)
	}

	used, err := r.usedOverlayIPs(ctx)
	if err != nil {
		return err
	}
	nodeWG, err := wg.AllocateClusterNodes(fcj, clusterName, used, r.Cfg.PoolStart, r.Cfg.PoolEnd, existingNodes)
	if err != nil {
		return err
	}
	if len(nodeWG) == 0 {
		return fmt.Errorf("no cluster nodes to allocate wireguard identities")
	}
	bootstrapNode := bootstrapControlNodeWG(fcj, nodeWG)
	addr := bootstrapNode.WGAddress
	priv := bootstrapNode.PrivateKey
	pub := bootstrapNode.PublicKey

	rocky := r.Cfg.RockyMajor(fcj.Spec.RockyVersion)
	bundleMode := r.clusterJoinBundleMode(fcj)
	enrollBase := r.Cfg.EnrollmentBaseURL

	scriptParams := script.Params{
		SiteName:              fcj.Spec.SiteId,
		ClusterName:           clusterName,
		EnrollmentToken:       enrollToken,
		HubWireguardIP:        r.Cfg.HubWireguardIP,
		EnrollmentBaseURL:     enrollBase,
		WGPublicKey:           pub,
		RockyMajor:            rocky,
		BundleMode:            bundleMode,
		WireguardEndpointHost: r.Cfg.WireguardEndpointHost,
		WireguardEndpointPort: r.Cfg.WireguardEndpointPort,
		KubevirtBridgeEnabled: fcj.Spec.WantsKubevirtBridge(),
	}
	if !bundleMode {
		scriptParams.WGConfigBlock = script.WGConfigBlock(
			priv, pub, addr, hubPub,
			r.Cfg.WireguardEndpointHost, r.Cfg.WireguardEndpointPort,
			r.Cfg.WireguardKeepalive, r.Cfg.WireguardSubnet,
		)
	}
	bootstrap, err := script.Render(scriptParams)
	if err != nil {
		return err
	}

	if err := r.ensureClusterInventoryCM(ctx, fcj, clusterName, nodeWG); err != nil {
		return err
	}
	sshKey, err := r.ensureClusterSSHSecret(ctx, fcj, clusterName)
	if err != nil {
		return err
	}
	if bundleMode && len(sshKey) == 0 {
		return fmt.Errorf("bootstrap bundle requires cluster SSH private key")
	}

	nodesWGJSON, err := wg.NodeWGToJSON(nodeWG)
	if err != nil {
		return err
	}
	secretData := map[string]string{
		"bootstrap.sh":     bootstrap,
		"enrollment-token": enrollToken,
		"nodes-wg.json":    string(nodesWGJSON),
		"wg-private-key":   priv,
		"wg-public-key":    pub,
	}

	wgZipConfigs := make([]bundle.WireguardConfig, 0, len(nodeWG))
	for _, n := range nodeWG {
		conf := script.WGConfigBlock(
			n.PrivateKey, n.PublicKey, n.WGAddress, hubPub,
			r.Cfg.WireguardEndpointHost, r.Cfg.WireguardEndpointPort,
			r.Cfg.WireguardKeepalive, r.Cfg.WireguardSubnet,
		)
		wgZipConfigs = append(wgZipConfigs, bundle.WireguardConfig{Hostname: n.Hostname, Content: []byte(conf)})
	}

	if bundleMode {
		ini := r.buildClusterInventoryINI(ctx, fcj, clusterName, nodeWG, false)
		hostBridgeScript := ""
		if fcj.Spec.WantsKubevirtBridge() {
			hb := fcj.Spec.Providers.KubevirtBridge
			var err error
			hostBridgeScript, err = script.RenderHostBridge(script.HostBridgeParams{
				Enabled:    true,
				BridgeName: hb.EffectiveBridgeName(),
				SlaveNic:   strings.TrimSpace(hb.SlaveNic),
				KeepIP:     hb.EffectiveKeepIP(),
			})
			if err != nil {
				return fmt.Errorf("render host bridge script: %w", err)
			}
		}
		archiveBytes, err := bundle.Build(bundle.Params{
			ClusterName:       clusterName,
			SiteID:            fcj.Spec.SiteId,
			InventoryINI:      ini,
			EnrollmentToken:   enrollToken,
			BootstrapScript:   bootstrap,
			HostBridgeScript:  hostBridgeScript,
			SSHPrivateKey:     sshKey,
			EnrollmentBaseURL: enrollBase,
			WireguardConfigs:  wgZipConfigs,
		})
		if err != nil {
			return fmt.Errorf("build bootstrap bundle: %w", err)
		}
		if fcj.Annotations == nil {
			fcj.Annotations = map[string]string{}
		}
		fcj.Annotations[bundle.AnnotationBundleMode] = "true"
		if err := r.Update(ctx, fcj); err != nil {
			return err
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: fcj.Namespace},
			Type:       corev1.SecretTypeOpaque,
			StringData: secretData,
			Data:       map[string][]byte{bundle.BootstrapArchiveKey: archiveBytes},
		}
		if err := controllerutil.SetControllerReference(fcj, secret, r.Scheme); err != nil {
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
			existing.Data = secret.Data
			if err := r.Update(ctx, &existing); err != nil {
				return err
			}
		}
	} else {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: fcj.Namespace},
			Type:       corev1.SecretTypeOpaque,
			StringData: secretData,
		}
		if err := controllerutil.SetControllerReference(fcj, secret, r.Scheme); err != nil {
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
	}

	wgPeers := make([]jobs.WGPeer, 0, len(nodeWG))
	newPubkeys := map[string]struct{}{}
	for _, n := range nodeWG {
		wgPeers = append(wgPeers, jobs.WGPeer{PublicKey: n.PublicKey, AllowedIP: wg.HostIP(n.WGAddress)})
		newPubkeys[n.PublicKey] = struct{}{}
	}
	removePeers := staleClusterWGPeers(existingNodes, fcj.Status.Nodes, newPubkeys)
	if err := r.ensureClusterWgSyncJob(ctx, fcj, clusterName, wgPeers, removePeers); err != nil {
		return err
	}

	if fcj.Status.Phase == "" || fcj.Status.Phase == fleetv1alpha1.JoinPhasePending {
		fcj.Status.Phase = fleetv1alpha1.JoinPhaseScriptIssued
	}
	if bundleMode {
		fcj.Status.Message = "Bootstrap bundle ready for download"
	} else {
		fcj.Status.Message = "Bootstrap script ready for download"
	}
	fcj.Status.WireguardAddress = addr
	fcj.Status.WireguardPublicKey = pub
	fcj.Status.Nodes = make([]fleetv1alpha1.FleetClusterJoinNodeStatus, 0, len(nodeWG))
	for _, n := range nodeWG {
		fcj.Status.Nodes = append(fcj.Status.Nodes, fleetv1alpha1.FleetClusterJoinNodeStatus{
			Hostname:           n.Hostname,
			Address:            n.Address,
			WireguardAddress:   n.WGAddress,
			WireguardPublicKey: n.PublicKey,
		})
	}
	return r.Status().Update(ctx, fcj)
}

func (r *ClusterJoinReconciler) clusterNodeWG(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) ([]wg.NodeWG, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: clusterBootstrapSecretName(clusterName), Namespace: fcj.Namespace}, &secret); err == nil {
		if nodes, err := wg.NodeWGFromJSON(secret.Data["nodes-wg.json"]); err == nil && len(nodes) > 0 {
			return nodes, nil
		}
	}
	if len(fcj.Status.Nodes) > 0 {
		out := make([]wg.NodeWG, 0, len(fcj.Status.Nodes))
		for _, n := range fcj.Status.Nodes {
			out = append(out, wg.NodeWG{
				Hostname:  n.Hostname,
				Address:   n.Address,
				WGAddress: n.WireguardAddress,
				PublicKey: n.WireguardPublicKey,
			})
		}
		return out, nil
	}
	if fcj.Status.WireguardAddress != "" {
		return []wg.NodeWG{{
			Hostname:  fcj.Spec.InitNodeHostname(clusterName),
			Address:   fcj.Spec.BootstrapHost,
			WGAddress: fcj.Status.WireguardAddress,
			PublicKey: fcj.Status.WireguardPublicKey,
		}}, nil
	}
	return nil, fmt.Errorf("cluster %s has no wireguard node records", clusterName)
}

func (r *ClusterJoinReconciler) startClusterValidate(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) error {
	nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
	if err != nil {
		return err
	}
	if err := r.ensureClusterInventoryCM(ctx, fcj, clusterName, nodeWG); err != nil {
		return err
	}
	if _, err := r.ensureClusterSSHSecret(ctx, fcj, clusterName); err != nil {
		return err
	}
	jobName := "fleet-cluster-validate-" + clusterName
	topology := fcj.Spec.Topology
	if topology == "" {
		topology = fleetv1alpha1.ClusterTopologySingleNode
	}
	j := jobs.ValidateJob(fcj.Namespace, clusterName, clusterName, fcj.Spec.SiteId, topology, r.Cfg.AnsibleRunnerImage,
		clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName))
	j.Name = jobName
	if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
		return err
	}
	_ = r.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: fcj.Namespace}},
		client.PropagationPolicy(metav1.DeletePropagationForeground))
	if err := r.Create(ctx, j); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	fcj.Status.ValidateJobName = jobName
	fcj.Status.Message = "Validation running"
	return r.Status().Update(ctx, fcj)
}

func (r *ClusterJoinReconciler) startClusterProvision(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) error {
	nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
	if err != nil {
		return err
	}
	if err := r.ensureClusterInventoryCM(ctx, fcj, clusterName, nodeWG); err != nil {
		return err
	}
	jobName := "fleet-cluster-provision-" + clusterName
	topology := fcj.Spec.Topology
	if topology == "" {
		topology = fleetv1alpha1.ClusterTopologySingleNode
	}
	j := jobs.ProvisionJob(fcj.Namespace, clusterName, clusterName, fcj.Spec.SiteId, topology,
		r.clusterJoinBundleMode(fcj), r.Cfg.AnsibleRunnerImage,
		clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName))
	j.Name = jobName
	if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
		return err
	}
	_ = r.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: fcj.Namespace}},
		client.PropagationPolicy(metav1.DeletePropagationForeground))
	if err := r.Create(ctx, j); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	fcj.Status.Phase = fleetv1alpha1.JoinPhaseProvisioning
	fcj.Status.ProvisionJobName = jobName
	fcj.Status.Message = "Provisioning cluster (Ansible)"
	return r.Status().Update(ctx, fcj)
}

func (r *ClusterJoinReconciler) ensureClusterInventoryCM(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string, nodeWG []wg.NodeWG) error {
	// Keep old CP in inventory for the whole replaceControlPlane job lifetime so
	// concurrent Ready-path inventory refreshes cannot drop the leave host.
	nodeWG = r.nodeWGWithReplaceControlOld(fcj, nil, nodeWG)
	ini := r.buildClusterInventoryINI(ctx, fcj, clusterName, nodeWG, true)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: clusterInventoryCMName(clusterName), Namespace: fcj.Namespace},
		Data:       map[string]string{"inventory.ini": ini},
	}
	if err := controllerutil.SetControllerReference(fcj, cm, r.Scheme); err != nil {
		return err
	}
	var ex corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, &ex); apierrors.IsNotFound(err) {
		return r.Create(ctx, cm)
	}
	ex.Data = cm.Data
	return r.Update(ctx, &ex)
}

func writeFleetClusterStorageVars(b *strings.Builder, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) {
	if fcj.Spec.Storage == nil {
		return
	}
	st := fcj.Spec.Storage
	b.WriteString(fmt.Sprintf("fleet_cluster_storage_backend=%s\n", st.EffectiveBackend()))
	b.WriteString(fmt.Sprintf("fleet_cluster_nfs_enabled=%s\n", boolLabel(st.NFSEnabled())))
	b.WriteString(fmt.Sprintf("fleet_cluster_longhorn_enabled=%s\n", boolLabel(st.LonghornEnabled())))
	b.WriteString(fmt.Sprintf("fleet_cluster_local_path_enabled=%s\n", boolLabel(st.LocalPathEnabled())))
	if st.LonghornEnabled() {
		b.WriteString(fmt.Sprintf("fleet_cluster_longhorn_device=%s\n", st.EffectiveLonghornDevice()))
		b.WriteString(fmt.Sprintf("fleet_cluster_longhorn_storage_class=%s\n", st.EffectiveLonghornStorageClass()))
		if st.MinDataDiskGi > 0 {
			b.WriteString(fmt.Sprintf("fleet_cluster_min_data_disk_gi=%d\n", st.MinDataDiskGi))
		}
	}
	if !st.NFSEnabled() {
		return
	}
	server, exportBase, clusterPath, storageClass, delegateHost := "", "/srv/fleet-nfs", clusterName, "nfs-client", ""
	if nfs := fcj.Spec.Storage.NFS; nfs != nil {
		server = nfs.Server
		if nfs.ExportBase != "" {
			exportBase = nfs.ExportBase
		}
		if nfs.ClusterPath != "" {
			clusterPath = nfs.ClusterPath
		}
		if nfs.StorageClass != "" {
			storageClass = nfs.StorageClass
		}
		delegateHost = nfs.DelegateHost
	}
	// nfs_local: node is the NFS server; default server to bootstrap host when unset.
	if st.NFSLocalEnabled() && server == "" {
		server = strings.TrimSpace(fcj.Spec.BootstrapHost)
	}
	if server == "" && !st.NFSLocalEnabled() {
		return
	}
	b.WriteString(fmt.Sprintf("cluster_storage_nfs_server=%s\n", server))
	b.WriteString(fmt.Sprintf("cluster_storage_nfs_export_base=%s\n", exportBase))
	b.WriteString(fmt.Sprintf("cluster_storage_nfs_cluster_path=%s\n", clusterPath))
	b.WriteString(fmt.Sprintf("cluster_storage_nfs_storage_class=%s\n", storageClass))
	if nfs := fcj.Spec.Storage.NFS; nfs != nil && nfs.MountPath != "" {
		b.WriteString(fmt.Sprintf("cluster_storage_nfs_mount_path=%s\n", nfs.MountPath))
	}
	if delegateHost != "" && !st.NFSLocalEnabled() {
		b.WriteString(fmt.Sprintf("cluster_storage_nfs_delegate_host=%s\n", storage.NFSDelegateInventoryHost()))
	}
}

func writeFleetNFSDelegateInventory(b *strings.Builder, fcj *fleetv1alpha1.FleetClusterJoin, hubRunner bool, sshUser string) {
	if fcj.Spec.Storage == nil || !fcj.Spec.Storage.NFSEnabled() {
		return
	}
	if fcj.Spec.Storage.NFS == nil || strings.TrimSpace(fcj.Spec.Storage.NFS.DelegateHost) == "" {
		return
	}
	host := strings.TrimSpace(fcj.Spec.Storage.NFS.DelegateHost)
	// Group name must differ from the host name. Ansible drops ansible_host on
	// delegate_to when a host and a group share a name.
	b.WriteString("\n[fleet_nfs]\n")
	b.WriteString(fmt.Sprintf("%s ansible_host=%s ansible_user=%s\n", storage.NFSDelegateInventoryHost(), host, sshUser))
	if hubRunner {
		b.WriteString("\n[fleet_nfs:vars]\n")
		b.WriteString("ansible_ssh_common_args=-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null\n")
		b.WriteString("ansible_ssh_private_key_file=/root/.ssh/id_rsa\n")
	}
}

func writeFleetClusterEdgeVars(b *strings.Builder, fcj *fleetv1alpha1.FleetClusterJoin, site *fleetv1alpha1.FleetSite) {
	ingressID := strings.TrimSpace(fcj.Spec.Edge.IngressId)
	if ingressID == "" && site != nil {
		ingressID = site.Spec.AutoIngressID()
	}
	method := ""
	tls := ""
	direct := false
	if site != nil && ingressID != "" {
		if ci, err := ingress.ResolveClusterIngress(site.Spec, ingressID); err == nil {
			method = ci.Method
			tls = ci.TlsPolicy
			direct = ci.AllowsSiteRoute
		}
	}
	b.WriteString(fmt.Sprintf("fleet_cluster_ingress_id=%s\n", ingressID))
	b.WriteString(fmt.Sprintf("fleet_cluster_exposure_mode=%s\n", method))
	b.WriteString(fmt.Sprintf("fleet_cluster_tls_policy=%s\n", tls))
	b.WriteString(fmt.Sprintf("fleet_cluster_spoke_direct_ingress=%s\n", boolLabel(direct)))
	b.WriteString("fleet_cluster_traefik_http_port=30080\n")
	b.WriteString("fleet_cluster_traefik_https_port=30443\n")
	b.WriteString("phase6_deploy_whoami_demo=false\n")
}

func (r *ClusterJoinReconciler) loadFleetSite(ctx context.Context, siteID string) *fleetv1alpha1.FleetSite {
	if siteID == "" || r.Reconciler == nil || r.Reconciler.Client == nil {
		return nil
	}
	fs, err := sites.FindFleetSite(ctx, r.Client, r.Cfg.Namespace, siteID)
	if err != nil {
		return nil
	}
	return fs
}

func (r *ClusterJoinReconciler) effectiveJumphost(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin) string {
	if j := strings.TrimSpace(fcj.Spec.Jumphost); j != "" {
		return j
	}
	if r.Reconciler == nil || r.Reconciler.Client == nil {
		return ""
	}
	siteID := strings.TrimSpace(fcj.Spec.SiteId)
	if siteID == "" {
		return ""
	}
	fs := r.loadFleetSite(ctx, siteID)
	if fs == nil {
		return ""
	}
	return strings.TrimSpace(fs.Spec.Jumphost)
}

func (r *ClusterJoinReconciler) inventorySSH(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, hubRunner bool) (sshArgs string, useMgmtLAN bool) {
	// Keepalives so a connection dropped by a node-side action (for example
	// firewalld starting and flushing nftables) fails fast instead of blackholing
	// and hanging the whole play.
	base := "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null " +
		"-o ServerAliveInterval=15 -o ServerAliveCountMax=3 -o ConnectTimeout=15"
	jh := r.effectiveJumphost(ctx, fcj)
	if hubRunner && jh != "" {
		return ssh.AnsibleSSHCommonArgs(base, jh, r.Cfg.SSHUser, "/root/.ssh/id_rsa"), true
	}
	return base, false
}

func nodeAnsibleHost(n wg.NodeWG, fallback string, hubRunner, useMgmtLAN bool) string {
	if hubRunner && !useMgmtLAN {
		if ip := wg.HostIP(n.WGAddress); ip != "" {
			return ip
		}
		// Injected replace-old hosts may lack WG keys; fall back to management address.
	}
	if n.Address != "" {
		return n.Address
	}
	return fallback
}

const clusterLabelConfigureFirewalld = "fleet.configure/firewalld"

func configureFirewalldEnabled(fcj *fleetv1alpha1.FleetClusterJoin) bool {
	if fcj == nil || fcj.Spec.ClusterLabels == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(fcj.Spec.ClusterLabels[clusterLabelConfigureFirewalld])) {
	case "true", "yes", "1":
		return true
	default:
		return false
	}
}

func writeConfigureFirewalldVar(b *strings.Builder, fcj *fleetv1alpha1.FleetClusterJoin) {
	if configureFirewalldEnabled(fcj) {
		b.WriteString("configure_firewalld=true\n")
	}
}

func (r *ClusterJoinReconciler) buildClusterInventoryINI(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string, nodeWG []wg.NodeWG, hubRunner bool) string {
	if len(nodeWG) == 0 {
		return ""
	}
	site := r.loadFleetSite(ctx, fcj.Spec.SiteId)
	bootstrap := bootstrapControlNodeWG(fcj, nodeWG)
	bootstrapWG := wg.HostIP(bootstrap.WGAddress)
	bootstrapMgmt := bootstrap.Address
	if bootstrapMgmt == "" {
		bootstrapMgmt = fcj.Spec.BootstrapHost
	}

	caps := fcj.Spec.Capabilities()
	enableKubevirt := caps["kvm"]
	kubevirtEnable := "false"
	if enableKubevirt {
		kubevirtEnable = "true"
	}
	sshKeyFile := "ssh/id_rsa"
	if hubRunner {
		sshKeyFile = "/root/.ssh/id_rsa"
	}
	sshArgs, useMgmtLAN := r.inventorySSH(ctx, fcj, hubRunner)
	minDisk := fcj.Spec.MinRootDiskGi
	if minDisk == 0 {
		minDisk = 40
	}

	writeNodeGroups := func(b *strings.Builder) {
		controls := controlNodesWG(nodeWG)
		workers := workerNodesWG(nodeWG)
		if len(controls) > 0 {
			b.WriteString("[fleet_cluster_control]\n")
			for _, n := range controls {
				sshHost := nodeAnsibleHost(n, fcj.Spec.BootstrapHost, hubRunner, useMgmtLAN)
				b.WriteString(fmt.Sprintf("%s ansible_host=%s ansible_user=%s wireguard_address=%s fleet_node_mgmt_address=%s k3s_role=server%s\n",
					n.Hostname, sshHost, r.Cfg.SSHUser, n.WGAddress, n.Address, nodeLabelsInventorySuffix(fcj, n.Hostname)))
			}
		}
		if len(workers) > 0 {
			b.WriteString("\n[fleet_cluster_workers]\n")
			for _, n := range workers {
				sshHost := nodeAnsibleHost(n, fcj.Spec.BootstrapHost, hubRunner, useMgmtLAN)
				b.WriteString(fmt.Sprintf("%s ansible_host=%s ansible_user=%s wireguard_address=%s fleet_node_mgmt_address=%s k3s_role=agent%s\n",
					n.Hostname, sshHost, r.Cfg.SSHUser, n.WGAddress, n.Address, nodeLabelsInventorySuffix(fcj, n.Hostname)))
			}
		}
		b.WriteString("\n[fleet_cluster_nodes:children]\n")
		if len(controls) > 0 {
			b.WriteString("fleet_cluster_control\n")
		}
		if len(workers) > 0 {
			b.WriteString("fleet_cluster_workers\n")
		}
	}

	if fcj.Spec.IsHA3Node() {
		initHost := bootstrap.Hostname
		if initHost == "" {
			initHost = fcj.Spec.InitNodeHostname(clusterName)
		}
		var b strings.Builder
		writeNodeGroups(&b)
		b.WriteString("\n[fleet_cluster_nodes:vars]\n")
		b.WriteString("fleet_node=true\n")
		writeConfigureFirewalldVar(&b, fcj)
		b.WriteString(fmt.Sprintf("ansible_ssh_common_args=%s\n", sshArgs))
		b.WriteString(fmt.Sprintf("ansible_ssh_private_key_file=%s\n", sshKeyFile))
		b.WriteString(fmt.Sprintf("fleet_cluster_kvm=%s\n", kubevirtEnable))
		b.WriteString(fmt.Sprintf("fleet_site_enable_kubevirt=%s\n", kubevirtEnable))
		writeFleetClusterKubevirtBridgeVars(&b, fcj)
		b.WriteString("k3s_cluster_mode=true\n")
		b.WriteString(fmt.Sprintf("k3s_cluster_init_host=%s\n", initHost))
		b.WriteString(fmt.Sprintf("k3s_cluster_name=%s\n", clusterName))
		if vip := strings.TrimSpace(fcj.Spec.ControlPlaneVip); vip != "" {
			b.WriteString(fmt.Sprintf("k3s_control_plane_vip=%s\n", vip))
			b.WriteString(fmt.Sprintf("k3s_cluster_join_server=https://%s:6443\n", vip))
		}
		writeFleetClusterK3sNetworkVars(&b, fcj)
		writeHarborBootstrapVars(&b, r.Cfg.HubWireguardIP)
		b.WriteString("harbor_enable_quay_mirror=false\n")
		b.WriteString(fmt.Sprintf("fleet_site_min_root_disk_gi=%d\n", minDisk))
		b.WriteString(fmt.Sprintf("fleet_hub_wireguard_ip=%s\n", r.Cfg.HubWireguardIP))
		writeFleetClusterWireguardEndpointVars(&b, r.Cfg)
		b.WriteString(fmt.Sprintf("ocm_cluster_name=%s\n", clusterName))
		b.WriteString(fmt.Sprintf("ocm_hub_apiserver=https://%s:6443\n", r.Cfg.HubWireguardIP))
		b.WriteString(fmt.Sprintf("ocm_hub_host=%s\n", r.Cfg.HubHostInventoryName))
		b.WriteString(fmt.Sprintf("fleet_site_hub_inventory_name=%s\n", r.Cfg.HubHostInventoryName))
		b.WriteString("ocm_hub=false\n")
		b.WriteString("k3s_command_path=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n")
		b.WriteString("k3s_kubeconfig=/etc/rancher/k3s/k3s.yaml\n")
		b.WriteString("ocm_kubeconfig=/etc/rancher/k3s/k3s.yaml\n")
		writeFleetClusterStorageVars(&b, fcj, clusterName)
		b.WriteString("\n[site_bootstrap]\n")
		sshHost := nodeAnsibleHost(bootstrap, bootstrapMgmt, hubRunner, useMgmtLAN)
		b.WriteString(fmt.Sprintf("%s ansible_host=%s ansible_user=%s\n", initHost, sshHost, r.Cfg.SSHUser))
		b.WriteString("\n[site_bootstrap:vars]\n")
		b.WriteString(fmt.Sprintf("wireguard_address=%s\n", bootstrap.WGAddress))
		b.WriteString(fmt.Sprintf("fleet_node_mgmt_address=%s\n", bootstrapMgmt))
		b.WriteString(fmt.Sprintf("fleet_site_name=%s\n", clusterName))
		b.WriteString(fmt.Sprintf("fleet_cluster_name=%s\n", clusterName))
		b.WriteString(fmt.Sprintf("fleet_site_id=%s\n", fcj.Spec.SiteId))
		b.WriteString(fmt.Sprintf("fleet_site_wg_ip=%s/32\n", bootstrapWG))
		b.WriteString(fmt.Sprintf("fleet_site_sp_endpoint=%s\n", bootstrapWG))
		writeFleetClusterK3sNetworkVars(&b, fcj)
		writeFleetClusterStorageVars(&b, fcj, clusterName)
		writeFleetClusterEdgeVars(&b, fcj, site)
		b.WriteString("k3s_kubeconfig=/etc/rancher/k3s/k3s.yaml\n")
		b.WriteString("ocm_kubeconfig=/etc/rancher/k3s/k3s.yaml\n")
		b.WriteString("k3s_command_path=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n")
		b.WriteString("\n[ocm_spoke_join:children]\n")
		b.WriteString("site_bootstrap\n")
		clusterSSHHost := bootstrapWG
		if useMgmtLAN {
			clusterSSHHost = bootstrapMgmt
		}
		writeClusterAliasInventory(&b, clusterName, clusterSSHHost, r.Cfg.SSHUser)
		b.WriteString("k3s_kubeconfig=/etc/rancher/k3s/k3s.yaml\n")
		b.WriteString("ocm_kubeconfig=/etc/rancher/k3s/k3s.yaml\n")
		b.WriteString(fmt.Sprintf("fleet_site=%s\n", fcj.Spec.SiteId))
		b.WriteString(fmt.Sprintf("cluster_region=%s\n", fcj.Spec.ClusterRegion))
		b.WriteString(fmt.Sprintf("wireguard_address=%s\n", bootstrap.WGAddress))
		writeHubInventorySection(&b, r, hubRunner, sshKeyFile, sshArgs)
		writeFleetNFSDelegateInventory(&b, fcj, hubRunner, r.Cfg.SSHUser)
		return b.String()
	}

	n := bootstrap
	wgIP := wg.HostIP(n.WGAddress)
	sshHost := nodeAnsibleHost(n, fcj.Spec.BootstrapHost, hubRunner, useMgmtLAN)
	if sshHost == "" {
		sshHost = fcj.Spec.BootstrapHost
	}
	var b strings.Builder
	writeNodeGroups(&b)
	b.WriteString("\n[fleet_cluster_nodes:vars]\n")
	b.WriteString("fleet_node=true\n")
	writeConfigureFirewalldVar(&b, fcj)
	b.WriteString(fmt.Sprintf("ansible_ssh_common_args=%s\n", sshArgs))
	b.WriteString(fmt.Sprintf("ansible_ssh_private_key_file=%s\n", sshKeyFile))
	b.WriteString("k3s_cluster_mode=false\n")
	b.WriteString(fmt.Sprintf("fleet_hub_wireguard_ip=%s\n", r.Cfg.HubWireguardIP))
	writeFleetClusterWireguardEndpointVars(&b, r.Cfg)
	b.WriteString(fmt.Sprintf("fleet_site_min_root_disk_gi=%d\n", minDisk))
	writeFleetClusterK3sNetworkVars(&b, fcj)
	writeHarborBootstrapVars(&b, r.Cfg.HubWireguardIP)
	b.WriteString("harbor_enable_quay_mirror=false\n")
	b.WriteString("k3s_command_path=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n")
	b.WriteString("k3s_kubeconfig=/etc/rancher/k3s/k3s.yaml\n")
	b.WriteString("ocm_kubeconfig=/etc/rancher/k3s/k3s.yaml\n")
	b.WriteString("\n[site_bootstrap]\n")
	b.WriteString(fmt.Sprintf("%s ansible_host=%s ansible_user=%s\n", n.Hostname, sshHost, r.Cfg.SSHUser))
	b.WriteString("\n[site_bootstrap:vars]\n")
	b.WriteString(fmt.Sprintf("ocm_cluster_name=%s\n", clusterName))
	b.WriteString(fmt.Sprintf("ocm_hub_apiserver=https://%s:6443\n", r.Cfg.HubWireguardIP))
	b.WriteString(fmt.Sprintf("ocm_hub_host=%s\n", r.Cfg.HubHostInventoryName))
	b.WriteString(fmt.Sprintf("fleet_site_hub_inventory_name=%s\n", r.Cfg.HubHostInventoryName))
	b.WriteString("ocm_hub=false\n")
	b.WriteString(fmt.Sprintf("fleet_site_name=%s\n", clusterName))
	b.WriteString(fmt.Sprintf("fleet_cluster_name=%s\n", clusterName))
	b.WriteString(fmt.Sprintf("fleet_site_id=%s\n", fcj.Spec.SiteId))
	b.WriteString(fmt.Sprintf("fleet_site_wg_cidr=%s\n", n.WGAddress))
	b.WriteString(fmt.Sprintf("fleet_site_wg_ip=%s\n", wgIP))
	b.WriteString(fmt.Sprintf("fleet_site_sp_endpoint=%s\n", wgIP))
	b.WriteString(fmt.Sprintf("wireguard_address=%s\n", n.WGAddress))
	b.WriteString(fmt.Sprintf("fleet_node_mgmt_address=%s\n", sshHost))
	b.WriteString(fmt.Sprintf("fleet_site_enable_kubevirt=%s\n", kubevirtEnable))
	b.WriteString(fmt.Sprintf("fleet_cluster_kvm=%s\n", kubevirtEnable))
	b.WriteString("fleet_site_auto_install_kubevirt_operator=false\n")
	writeFleetClusterKubevirtBridgeVars(&b, fcj)
	writeFleetClusterK3sNetworkVars(&b, fcj)
	writeFleetClusterStorageVars(&b, fcj, clusterName)
	writeFleetClusterEdgeVars(&b, fcj, site)
	b.WriteString("k3s_kubeconfig=/etc/rancher/k3s/k3s.yaml\n")
	b.WriteString("ocm_kubeconfig=/etc/rancher/k3s/k3s.yaml\n")
	b.WriteString("\n[ocm_spoke_join:children]\n")
	b.WriteString("site_bootstrap\n")
	writeClusterAliasInventory(&b, clusterName, sshHost, r.Cfg.SSHUser)
	b.WriteString("k3s_kubeconfig=/etc/rancher/k3s/k3s.yaml\n")
	b.WriteString("ocm_kubeconfig=/etc/rancher/k3s/k3s.yaml\n")
	b.WriteString(fmt.Sprintf("fleet_site=%s\n", fcj.Spec.SiteId))
	b.WriteString(fmt.Sprintf("cluster_region=%s\n", fcj.Spec.ClusterRegion))
	b.WriteString(fmt.Sprintf("wireguard_address=%s\n", n.WGAddress))
	writeHubInventorySection(&b, r, hubRunner, sshKeyFile, sshArgs)
	writeFleetNFSDelegateInventory(&b, fcj, hubRunner, r.Cfg.SSHUser)
	return b.String()
}

// writeClusterAliasInventory adds the cluster name as an Ansible host without
// also creating a group of that name. Ansible drops ansible_host on delegate_to
// when a host and a group share a name (OCM CSR approval SSHes to this host).
func writeClusterAliasInventory(b *strings.Builder, clusterName, sshHost, sshUser string) {
	b.WriteString("\n[fleet_cluster]\n")
	b.WriteString(fmt.Sprintf("%s ansible_host=%s ansible_user=%s\n", clusterName, sshHost, sshUser))
	b.WriteString("\n[fleet_cluster:vars]\n")
}

// writeHubInventorySection emits [hub] for cluster Ansible jobs. Hub-runner pods run
// on the hub with hostNetwork; delegate tasks SSH to 127.0.0.1 (clusteradm, kubectl on host).
// Jumphost ProxyCommand applies only to spoke nodes, never to [hub].
func writeHubInventorySection(b *strings.Builder, r *ClusterJoinReconciler, hubRunner bool, sshKeyFile, sshArgs string) {
	b.WriteString("\n[hub]\n")
	if hubRunner {
		b.WriteString(fmt.Sprintf("%s ansible_host=127.0.0.1 ansible_user=%s\n", r.Cfg.HubHostInventoryName, r.Cfg.SSHUser))
	} else {
		b.WriteString(fmt.Sprintf("%s ansible_host=10.10.0.1 ansible_user=%s\n", r.Cfg.HubHostInventoryName, r.Cfg.SSHUser))
	}
	b.WriteString("\n[hub:vars]\n")
	b.WriteString("wireguard_address=10.10.0.1/24\n")
	b.WriteString("ocm_kubeconfig=/etc/rancher/k3s/k3s.yaml\n")
	b.WriteString("k3s_command_path=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n")
	if hubRunner {
		b.WriteString(fmt.Sprintf("ansible_ssh_private_key_file=%s\n", sshKeyFile))
		b.WriteString("ansible_ssh_common_args=-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null\n")
		b.WriteString("ocm_spoke_kubeconfig_delegate_local=true\n")
		b.WriteString(fmt.Sprintf("ocm_spoke_kubeconfig_ssh_private_key_file=%s\n", sshKeyFile))
		b.WriteString("ocm_spoke_kubeconfig_kubectl_config=/etc/kube/config\n")
		return
	}
	b.WriteString(fmt.Sprintf("ansible_ssh_private_key_file=%s\n", sshKeyFile))
	b.WriteString(fmt.Sprintf("ansible_ssh_common_args=%s\n", sshArgs))
}

func writeHarborBootstrapVars(b *strings.Builder, hubIP string) {
	host := strings.TrimSpace(hubIP)
	if host == "" {
		host = "10.10.0.1"
	}
	b.WriteString(fmt.Sprintf("harbor_host=%s\n", host))
	b.WriteString("harbor_nodeport_https=30443\n")
}

func writeFleetClusterKubevirtBridgeVars(b *strings.Builder, fcj *fleetv1alpha1.FleetClusterJoin) {
	hb := fcj.Spec.Providers.KubevirtBridge
	enabled := fcj.Spec.WantsKubevirtBridge()
	if enabled {
		b.WriteString("fleet_cluster_kubevirt_bridge_enabled=true\n")
		b.WriteString(fmt.Sprintf("kubevirt_bridge_host_bridge=%s\n", hb.EffectiveBridgeName()))
		b.WriteString("kubevirt_bridge_nad_name=mgmt-l2\n")
		b.WriteString("kubevirt_bridge_nad_namespace=default\n")
		if nic := strings.TrimSpace(hb.SlaveNic); nic != "" {
			b.WriteString(fmt.Sprintf("fleet_cluster_kubevirt_bridge_slave_nic=%s\n", nic))
		}
		if hb.EffectiveKeepIP() {
			b.WriteString("fleet_cluster_kubevirt_bridge_keep_ip=true\n")
		} else {
			b.WriteString("fleet_cluster_kubevirt_bridge_keep_ip=false\n")
		}
	} else {
		b.WriteString("fleet_cluster_kubevirt_bridge_enabled=false\n")
	}
}

func writeFleetClusterK3sNetworkVars(b *strings.Builder, fcj *fleetv1alpha1.FleetClusterJoin) {
	// When the host bridge moves mgmt IP off the slave NIC, Calico must autodetect the bridge interface.
	if fcj != nil && fcj.Spec.WantsKubevirtBridge() {
		hb := fcj.Spec.Providers.KubevirtBridge
		b.WriteString(fmt.Sprintf("calico_ip_autodetection_method=interface=%s\n", hb.EffectiveBridgeName()))
	} else {
		// auto: bundle Calico role detects the management LAN NIC per node (see roles/calico/tasks/detect_iface.yml).
		b.WriteString("calico_ip_autodetection_method=auto\n")
	}
	b.WriteString(fmt.Sprintf("fleet_cluster_wireguard_interface=%s\n", wg.OverlayInterfaceName))
	b.WriteString("k3s_bind_address=0.0.0.0\n")
	b.WriteString("k3s_node_ip={{ fleet_node_mgmt_address | default(ansible_host) }}\n")
	b.WriteString("k3s_advertise_address={{ fleet_node_mgmt_address | default(ansible_host) }}\n")
}

func writeFleetClusterWireguardEndpointVars(b *strings.Builder, cfg config.Config) {
	if host := strings.TrimSpace(cfg.WireguardEndpointHost); host != "" {
		b.WriteString(fmt.Sprintf("wireguard_endpoint_host=%s\n", host))
	}
	if port := strings.TrimSpace(cfg.WireguardEndpointPort); port != "" {
		b.WriteString(fmt.Sprintf("wireguard_endpoint_port=%s\n", port))
	}
}

func nodeWGIsWorker(n wg.NodeWG) bool {
	return n.Role == fleetv1alpha1.NodeRoleWorker
}

func controlNodesWG(nodes []wg.NodeWG) []wg.NodeWG {
	out := make([]wg.NodeWG, 0)
	for _, n := range nodes {
		if !nodeWGIsWorker(n) {
			out = append(out, n)
		}
	}
	return out
}

func workerNodesWG(nodes []wg.NodeWG) []wg.NodeWG {
	out := make([]wg.NodeWG, 0)
	for _, n := range nodes {
		if nodeWGIsWorker(n) {
			out = append(out, n)
		}
	}
	return out
}

func firstControlNodeWG(nodes []wg.NodeWG) wg.NodeWG {
	for _, n := range nodes {
		if !nodeWGIsWorker(n) {
			return n
		}
	}
	return nodes[0]
}

func bootstrapControlNodeWG(fcj *fleetv1alpha1.FleetClusterJoin, nodes []wg.NodeWG) wg.NodeWG {
	want := ""
	if fcj != nil {
		want = strings.TrimSpace(fcj.Spec.BootstrapHost)
	}
	if want != "" {
		for _, n := range nodes {
			if nodeWGIsWorker(n) {
				continue
			}
			if strings.TrimSpace(n.Address) == want {
				return n
			}
		}
	}
	return firstControlNodeWG(nodes)
}

func nodeLabelsForHost(fcj *fleetv1alpha1.FleetClusterJoin, hostname string) map[string]string {
	for _, n := range fcj.Spec.Nodes {
		h := n.Hostname
		if h == "" {
			continue
		}
		if h == hostname && len(n.Labels) > 0 {
			return n.Labels
		}
	}
	return nil
}

func nodeLabelsInventorySuffix(fcj *fleetv1alpha1.FleetClusterJoin, hostname string) string {
	labels := nodeLabelsForHost(fcj, hostname)
	if len(labels) == 0 {
		return ""
	}
	raw, err := json.Marshal(labels)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(" fleet_node_labels='%s'", string(raw))
}

func (r *ClusterJoinReconciler) ensureClusterSSHSecret(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) ([]byte, error) {
	name := clusterSSHSecretName(clusterName)
	var ex corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: fcj.Namespace}, &ex); err == nil {
		if len(ex.Data["id_rsa"]) > 0 {
			return ex.Data["id_rsa"], nil
		}
	}
	const shared = "fleet-site-ansible-ssh"
	var sharedSec corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: shared, Namespace: fcj.Namespace}, &sharedSec); err == nil && len(sharedSec.Data["id_rsa"]) > 0 {
		key := append([]byte(nil), sharedSec.Data["id_rsa"]...)
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: fcj.Namespace},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{"id_rsa": key},
		}
		if err := controllerutil.SetControllerReference(fcj, secret, r.Scheme); err != nil {
			return nil, err
		}
		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: fcj.Namespace}, &ex); apierrors.IsNotFound(err) {
			if err := r.Create(ctx, secret); err != nil {
				return nil, err
			}
			return key, nil
		}
		ex.Data = secret.Data
		if err := r.Update(ctx, &ex); err != nil {
			return nil, err
		}
		return key, nil
	}
	if r.clusterJoinBundleMode(fcj) {
		return nil, fmt.Errorf("cluster SSH key missing: create Secret %s with id_rsa or populate %s", shared, name)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: fcj.Namespace},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"README": "Populate id_rsa or create Secret fleet-site-ansible-ssh with the bootstrap SSH key",
		},
	}
	_ = controllerutil.SetControllerReference(fcj, secret, r.Scheme)
	if err := client.IgnoreAlreadyExists(r.Create(ctx, secret)); err != nil {
		return nil, err
	}
	return nil, nil
}

func (r *ClusterJoinReconciler) bootstrapBundleHasSSHKey(ctx context.Context, clusterName string) (bool, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: clusterBootstrapSecretName(clusterName), Namespace: r.Cfg.Namespace}, &secret); err != nil {
		return false, err
	}
	archiveBytes := secret.Data[bundle.BootstrapArchiveKey]
	if len(archiveBytes) == 0 {
		return false, nil
	}
	has, err := bundle.ArchiveContains(archiveBytes, "ssh/id_rsa")
	return has, err
}

func (r *ClusterJoinReconciler) finishClusterReady(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) error {
	if err := r.patchClusterLabels(ctx, fcj, clusterName); err != nil {
		return err
	}
	if err := r.triggerKubeconfigSync(ctx, fcj); err != nil {
		return err
	}
	if !gpuCapabilityDiscoverDeferred(fcj) {
		if err := r.triggerCapabilityDiscover(ctx, fcj); err != nil {
			return err
		}
	}
	if err := r.upsertEdgeClusterEntry(ctx, fcj, clusterName); err != nil {
		return err
	}
	cfg := registry.DomainConfig{
		FleetSiteNamespace:      r.Cfg.Namespace,
		EdgeDomainsCMNamespace:  r.Cfg.EdgeDomainsCMNamespace,
		EdgeDomainsCMName:       r.Cfg.EdgeDomainsCMName,
		EdgeClustersCMNamespace: r.Cfg.EdgeClustersCMNamespace,
		EdgeClustersCMName:      r.Cfg.EdgeClustersCMName,
		HubEdgeDomain:           r.Cfg.HubEdgeDomain,
		ManagementSiteID:        r.Cfg.ManagementSiteID,
	}
	_ = registry.SyncSiteAssignedClusters(ctx, r.Client, cfg)
	if err := registry.ReconcileEdgeDomains(ctx, r.Client, cfg); err != nil {
		return err
	}
	return r.applyInventoryInfrastructureDefaults(ctx, fcj, clusterName)
}

func (r *ClusterJoinReconciler) triggerIngressAuthReconcile(ctx context.Context) error {
	fia, err := ingressauth.EnsureCR(ctx, r.Client, r.Cfg.Namespace, r.Cfg)
	if err != nil {
		return err
	}
	if fia.Annotations == nil {
		fia.Annotations = map[string]string{}
	}
	fia.Annotations[ingressauth.ReconcileAnnotation] = time.Now().UTC().Format(time.RFC3339)
	return r.Update(ctx, fia)
}

func (r *ClusterJoinReconciler) triggerCapabilityDiscover(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin) error {
	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	if _, ok := fcj.Annotations[discoverAnnotation]; ok {
		return nil
	}
	if _, ok := fcj.Annotations["fleet.umkim.com/capability-discover-job"]; ok {
		return nil
	}
	fcj.Annotations[discoverAnnotation] = time.Now().UTC().Format(time.RFC3339)
	return r.Update(ctx, fcj)
}

func (r *ClusterJoinReconciler) patchClusterLabels(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) error {
	var mc unstructured.Unstructured
	mc.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	if err := r.Get(ctx, types.NamespacedName{Name: clusterName}, &mc); err != nil {
		return err
	}
	labels := mc.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels["fleet.site"] = fcj.Spec.SiteId
	registry.ApplyCapabilityPatch(labels, fcj.Spec.Capabilities())
	if fcj.Spec.ClusterRegion != "" {
		labels["fleet.topology/region"] = fcj.Spec.ClusterRegion
	}
	labels["fleet.topology/nest_depth"] = "0"
	labels["fleet.topology/provisioner"] = "k3s"
	if fcj.Spec.Topology != "" {
		labels["fleet.topology/topology"] = fcj.Spec.Topology
	}
	for k, v := range fcj.Spec.ClusterLabels {
		if allowedClusterLabel(k) {
			labels[k] = v
		}
	}
	mc.SetLabels(labels)
	return r.Update(ctx, &mc)
}

func allowedClusterLabel(k string) bool {
	return strings.HasPrefix(k, "fleet.custom/") || strings.HasPrefix(k, "fleet.")
}

func boolLabel(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func (r *ClusterJoinReconciler) upsertEdgeClusterEntry(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) error {
	ns := r.Cfg.EdgeClustersCMNamespace
	name := r.Cfg.EdgeClustersCMName
	if ns == "" || name == "" {
		return nil
	}
	type edgeEntry struct {
		Site            string `yaml:"site"`
		Management      bool   `yaml:"management,omitempty"`
		ReadOnly        bool   `yaml:"read_only,omitempty"`
		DirectIngress   bool   `yaml:"direct_ingress"`
		IngressID       string `yaml:"ingress_id,omitempty"`
		Method          string `yaml:"method,omitempty"`
		TlsPolicy       string `yaml:"tls_policy,omitempty"`
		DNSZone         string `yaml:"dns_zone,omitempty"`
		EdgeWANIP       string `yaml:"edge_wan_ip,omitempty"`
		WanAddressMode  string `yaml:"wan_address_mode,omitempty"`
		UpstreamHost    string `yaml:"upstream_host"`
		TraefikHTTPPort int    `yaml:"traefik_http_port"`
	}
	type edgeFile struct {
		Clusters map[string]edgeEntry `yaml:"clusters"`
	}
	var f edgeFile
	var existing corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &existing); err == nil {
		if raw := existing.Data["clusters.yaml"]; raw != "" {
			_ = yaml.Unmarshal([]byte(raw), &f)
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	if f.Clusters == nil {
		f.Clusters = map[string]edgeEntry{}
	}
	prev := edgeClusterUpsert{}
	if existingEnt, ok := f.Clusters[clusterName]; ok {
		prev = edgeClusterUpsert{
			Site:            existingEnt.Site,
			Management:      existingEnt.Management,
			ReadOnly:        existingEnt.ReadOnly,
			DirectIngress:   existingEnt.DirectIngress,
			Method:          existingEnt.Method,
			TlsPolicy:       existingEnt.TlsPolicy,
			IngressID:       existingEnt.IngressID,
			DNSZone:         existingEnt.DNSZone,
			EdgeWANIP:       existingEnt.EdgeWANIP,
			WanAddressMode:  existingEnt.WanAddressMode,
			UpstreamHost:    existingEnt.UpstreamHost,
			TraefikHTTPPort: existingEnt.TraefikHTTPPort,
		}
	}
	site := r.loadFleetSite(ctx, fcj.Spec.SiteId)
	upstream := wg.HostIP(fcj.Status.WireguardAddress)
	if fcj.Spec.IsLAN() && fcj.Spec.BootstrapHost != "" {
		upstream = fcj.Spec.BootstrapHost
	}
	merged := mergeEdgeClusterEntryForJoin(prev, fcj, site, upstream)
	f.Clusters[clusterName] = edgeEntry{
		Site:            merged.Site,
		Management:      merged.Management,
		ReadOnly:        merged.ReadOnly,
		DirectIngress:   merged.DirectIngress,
		IngressID:       merged.IngressID,
		Method:          merged.Method,
		TlsPolicy:       merged.TlsPolicy,
		DNSZone:         merged.DNSZone,
		EdgeWANIP:       merged.EdgeWANIP,
		WanAddressMode:  merged.WanAddressMode,
		UpstreamHost:    merged.UpstreamHost,
		TraefikHTTPPort: merged.TraefikHTTPPort,
	}
	out, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "fleet-edge",
				"app.kubernetes.io/component": "cluster-registry",
			},
		},
		Data: map[string]string{
			"clusters.yaml": string(out),
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data["clusters.yaml"] = string(out)
		if cm.Annotations == nil {
			cm.Annotations = map[string]string{}
		}
		cm.Annotations[registry.AnnotationConsoleManaged] = "true"
		return nil
	})
	return err
}

func (r *ClusterJoinReconciler) failClusterJoin(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, err error) (ctrl.Result, error) {
	fcj.Status.Phase = fleetv1alpha1.JoinPhaseFailed
	fcj.Status.Message = err.Error()
	_ = r.Status().Update(ctx, fcj)
	return ctrl.Result{}, err
}

func provisionFinalizeRetriable(message string) bool {
	if message == "" {
		return false
	}
	if strings.Contains(message, "Provision finalize failed") {
		return true
	}
	return strings.Contains(message, "object has been modified") ||
		strings.Contains(message, "fleet-edge-domains") ||
		strings.Contains(message, "fleet-edge-clusters")
}

func (r *ClusterJoinReconciler) clusterProvisionRecoverable(ctx context.Context, clusterName string) bool {
	avail, err := r.managedClusterAvailable(ctx, clusterName)
	if err != nil || !avail {
		return false
	}
	// Do not mark Ready until spoke kubeconfig is synced (console node list needs it).
	ready, err := r.spokeKubeconfigSecretReady(ctx, clusterName)
	return err == nil && ready
}

func lanEnrollmentBaseURL(hubLanAPI, fallback string) string {
	if hubLanAPI == "" {
		return fallback
	}
	u, err := url.Parse(hubLanAPI)
	if err != nil || u.Hostname() == "" {
		return fallback
	}
	return fmt.Sprintf("http://%s:30912/fleet/api/v1alpha1", u.Hostname())
}

func (r *ClusterJoinReconciler) clusterJoinBundleMode(fcj *fleetv1alpha1.FleetClusterJoin) bool {
	if fcj.Annotations != nil && fcj.Annotations[bundle.AnnotationBundleMode] == "true" {
		return true
	}
	return !fcj.Spec.IsLAN()
}

func clusterBootstrapSecretName(clusterName string) string {
	return "fleet-cluster-" + clusterName + "-bootstrap"
}
func clusterSSHSecretName(clusterName string) string {
	return "fleet-cluster-" + clusterName + "-ansible-ssh"
}
func clusterInventoryCMName(clusterName string) string {
	return "fleet-cluster-" + clusterName + "-inventory"
}

func staleClusterWGPeers(existing []wg.NodeWG, status []fleetv1alpha1.FleetClusterJoinNodeStatus, keep map[string]struct{}) []jobs.WGPeer {
	seen := map[string]struct{}{}
	out := make([]jobs.WGPeer, 0)
	add := func(pub string) {
		if pub == "" {
			return
		}
		if _, ok := keep[pub]; ok {
			return
		}
		if _, dup := seen[pub]; dup {
			return
		}
		seen[pub] = struct{}{}
		out = append(out, jobs.WGPeer{PublicKey: pub})
	}
	for _, n := range existing {
		add(n.PublicKey)
	}
	for _, n := range status {
		add(n.WireguardPublicKey)
	}
	return out
}
