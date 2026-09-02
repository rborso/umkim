package reconcile

import (
	"context"
	"fmt"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	"github.com/rborso/umkim/controllers/fleet-site/internal/providers"
	"github.com/rborso/umkim/controllers/fleet-site/internal/wg"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func providerAnnotation(kind, action string) string {
	return fmt.Sprintf("fleet.umkim.com/provider-%s-%s", kind, action)
}

func (r *ClusterJoinReconciler) reconcileClusterProviders(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	// v0.2: no DCM k8s/helm/kubevirt service providers. Ready is the deployable state.
	return ctrl.Result{}, nil
}

func allProviderKinds() []string {
	return []string{
		fleetv1alpha1.ProviderKindKubevirt,
		fleetv1alpha1.ProviderKindK8s,
		fleetv1alpha1.ProviderKindHelm,
	}
}

func (r *ClusterJoinReconciler) reconcileAutoProviderInstall(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		return ctrl.Result{}, nil
	}
	if busy, _ := r.providerJobBusy(fcj); busy {
		return ctrl.Result{}, nil
	}
	for _, kind := range fleetv1alpha1.ProviderInstallOrder {
		if !fcj.Spec.WantsProviderInstall(kind) {
			continue
		}
		st := fcj.Status.Providers[kind]
		phase := st.Phase
		if phase == "" {
			phase = fleetv1alpha1.ProviderPhaseNotInstalled
		}
		if phase != fleetv1alpha1.ProviderPhaseNotInstalled {
			continue
		}
		if res, err := r.startClusterProviderJob(ctx, fcj, clusterName, kind, "install"); err != nil {
			st.Phase = fleetv1alpha1.ProviderPhaseFailed
			st.Message = err.Error()
			fcj.Status.Providers[kind] = st
			_ = r.Status().Update(ctx, fcj)
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		} else if res.Requeue || res.RequeueAfter > 0 {
			return res, nil
		}
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func (r *ClusterJoinReconciler) providerJobBusy(fcj *fleetv1alpha1.FleetClusterJoin) (bool, string) {
	if fcj.Status.Providers == nil {
		return false, ""
	}
	for kind, st := range fcj.Status.Providers {
		if st.JobName != "" && (st.Phase == fleetv1alpha1.ProviderPhaseInstalling || st.Phase == fleetv1alpha1.ProviderPhaseDeleting) {
			return true, kind
		}
	}
	return false, ""
}

func (r *ClusterJoinReconciler) ensureProviderNodePorts(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, kind string) error {
	install := fleetv1alpha1.ProviderInstallSpec{}
	changed := false
	if fcj.Spec.Providers.K8sNodePort == 0 {
		install.K8s = true
		changed = true
	}
	if fcj.Spec.Providers.HelmNodePort == 0 {
		install.Helm = true
		changed = true
	}
	if kind == fleetv1alpha1.ProviderKindKubevirt && fcj.Spec.Providers.KubevirtSPNodeport == 0 {
		install.Kubevirt = true
		changed = true
	}
	if !changed {
		return nil
	}
	ports, err := providers.AllocateNodePorts(ctx, r.Client, fcj.Namespace, install)
	if err != nil {
		return err
	}
	if fcj.Spec.Providers.K8sNodePort == 0 && ports.K8s > 0 {
		fcj.Spec.Providers.K8sNodePort = ports.K8s
	}
	if fcj.Spec.Providers.HelmNodePort == 0 && ports.Helm > 0 {
		fcj.Spec.Providers.HelmNodePort = ports.Helm
	}
	if fcj.Spec.Providers.KubevirtSPNodeport == 0 && ports.Kubevirt > 0 {
		fcj.Spec.Providers.KubevirtSPNodeport = ports.Kubevirt
	}
	return r.Update(ctx, fcj)
}

func (r *ClusterJoinReconciler) startClusterProviderJob(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName, kind, action string) (ctrl.Result, error) {
	if action == "install" {
		if err := r.ensureProviderNodePorts(ctx, fcj, kind); err != nil {
			return ctrl.Result{}, err
		}
	}
	nodeWG, err := r.clusterNodeWG(ctx, fcj, clusterName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureClusterInventoryCM(ctx, fcj, clusterName, nodeWG); err != nil {
		return ctrl.Result{}, err
	}
	if _, err := r.ensureClusterSSHSecret(ctx, fcj, clusterName); err != nil {
		return ctrl.Result{}, err
	}
	spEndpoint := clusterSpEndpoint(fcj, nodeWG)
	jobName := fmt.Sprintf("fleet-cluster-provider-%s-%s-%s", kind, action, clusterName)
	extra := providerJobExtraVars(fcj, kind, spEndpoint, r.Cfg.HubWireguardIP)
	j := jobs.ProviderJob(fcj.Namespace, jobName, clusterName, fcj.Spec.SiteId, kind, action,
		r.Cfg.AnsibleRunnerImage, clusterSSHSecretName(clusterName), clusterInventoryCMName(clusterName), extra)
	if err := controllerutil.SetControllerReference(fcj, j, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureProviderJob(ctx, j); err != nil {
		if isJobTerminating(err) {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}
	if fcj.Status.Providers == nil {
		fcj.Status.Providers = map[string]fleetv1alpha1.ClusterProviderStatus{}
	}
	st := fcj.Status.Providers[kind]
	st.JobName = jobName
	st.ProviderName = providerNameForKind(fcj, clusterName, kind)
	if action == "delete" {
		st.Phase = fleetv1alpha1.ProviderPhaseDeleting
		st.Message = fmt.Sprintf("Removing %s provider", kind)
	} else {
		st.Phase = fleetv1alpha1.ProviderPhaseInstalling
		st.Message = fmt.Sprintf("Installing %s provider", kind)
	}
	fcj.Status.Providers[kind] = st
	fcj.Status.Message = st.Message
	return ctrl.Result{}, r.Status().Update(ctx, fcj)
}

func providerNameForKind(fcj *fleetv1alpha1.FleetClusterJoin, clusterName, kind string) string {
	switch kind {
	case fleetv1alpha1.ProviderKindHelm:
		return fcj.Spec.HelmProviderName(clusterName)
	case fleetv1alpha1.ProviderKindKubevirt:
		return fcj.Spec.KubevirtProviderName(clusterName)
	default:
		return fcj.Spec.K8sProviderName(clusterName)
	}
}

func clusterSpEndpoint(fcj *fleetv1alpha1.FleetClusterJoin, nodeWG []wg.NodeWG) string {
	if fcj.Status.WireguardAddress != "" {
		return strings.Split(fcj.Status.WireguardAddress, "/")[0]
	}
	if len(nodeWG) > 0 {
		return wg.HostIP(nodeWG[0].WGAddress)
	}
	return fcj.Spec.BootstrapHost
}

func providerJobExtraVars(fcj *fleetv1alpha1.FleetClusterJoin, kind, spEndpoint, hubWG string) []string {
	clusterName := fcj.Spec.EffectiveClusterName(fcj.Name)
	out := []string{
		"-e", "fleet_site_sp_endpoint=" + spEndpoint,
		"-e", "fleet_cluster_name=" + clusterName,
		"-e", "ocm_cluster_name=" + clusterName,
		"-e", "fleet_hub_wireguard_ip=" + hubWG,
	}
	switch kind {
	case fleetv1alpha1.ProviderKindHelm:
		if fcj.Spec.Providers.HelmNodePort > 0 {
			out = append(out, "-e", fmt.Sprintf("fleet_site_dcm_helm_sp_nodeport=%d", fcj.Spec.Providers.HelmNodePort))
		}
		out = append(out, "-e", "fleet_site_dcm_helm_sp_name="+fcj.Spec.HelmProviderName(clusterName))
	case fleetv1alpha1.ProviderKindK8s:
		if fcj.Spec.Providers.K8sNodePort > 0 {
			out = append(out, "-e", fmt.Sprintf("fleet_site_dcm_k8s_sp_nodeport=%d", fcj.Spec.Providers.K8sNodePort))
		}
		out = append(out, "-e", "fleet_site_dcm_k8s_sp_name="+fcj.Spec.K8sProviderName(clusterName))
	case fleetv1alpha1.ProviderKindKubevirt:
		// fleet_site.yml defaults enable_kubevirt=false; must override for provider install jobs.
		out = append(out, "-e", "fleet_site_enable_kubevirt=true")
		out = append(out, "-e", "fleet_cluster_kvm=true")
		if fcj.Spec.Providers.KubevirtSPNodeport > 0 {
			out = append(out, "-e", fmt.Sprintf("fleet_site_dcm_kubevirt_sp_nodeport=%d", fcj.Spec.Providers.KubevirtSPNodeport))
		}
		out = append(out, "-e", "fleet_site_dcm_kubevirt_sp_name="+fcj.Spec.KubevirtProviderName(clusterName))
		if fcj.Spec.WantsKubevirtBridge() {
			hb := fcj.Spec.Providers.KubevirtBridge
			out = append(out, "-e", "fleet_cluster_kubevirt_bridge_enabled=true")
			out = append(out, "-e", "kubevirt_bridge_host_bridge="+hb.EffectiveBridgeName())
			out = append(out, "-e", "kubevirt_bridge_nad_name=mgmt-l2")
			out = append(out, "-e", "kubevirt_bridge_nad_namespace=default")
		} else {
			out = append(out, "-e", "fleet_cluster_kubevirt_bridge_enabled=false")
		}
	}
	return out
}

func ensureProviderStatusDefaults(fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) {
	if fcj.Status.Providers == nil {
		fcj.Status.Providers = map[string]fleetv1alpha1.ClusterProviderStatus{}
	}
	for _, kind := range allProviderKinds() {
		if _, ok := fcj.Status.Providers[kind]; !ok {
			fcj.Status.Providers[kind] = fleetv1alpha1.ClusterProviderStatus{
				Phase:        fleetv1alpha1.ProviderPhaseNotInstalled,
				ProviderName: providerNameForKind(fcj, clusterName, kind),
			}
		}
	}
}
