package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/overlay"
	"github.com/rborso/umkim/controllers/fleet-site/internal/providers"
	"github.com/rborso/umkim/controllers/fleet-site/internal/wg"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const adoptAnnotation = "fleet.umkim.com/adopted-inventory"

type clusterAdoptBody struct {
	SiteId        string `json:"siteId,omitempty"`
	BootstrapHost string `json:"bootstrapHost,omitempty"`
	WireguardIP   string `json:"wireguardIp,omitempty"`
	K8sNodePort   int    `json:"k8sNodePort,omitempty"`
	HelmNodePort  int    `json:"helmNodePort,omitempty"`
	KubevirtPort  int    `json:"kubevirtSpNodeport,omitempty"`
}

func (s *Server) handleAdoptCluster(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req clusterAdoptBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if s.clusterReadOnly(name, edge) {
		http.Error(w, "cluster "+name+" is read-only", http.StatusForbidden)
		return
	}

	mc, err := getManagedCluster(ctx, s.Client, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster not registered in OCM", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !mcConditionAvailable(mc) {
		http.Error(w, "ManagedCluster must be Available before adopt", http.StatusConflict)
		return
	}
	if _, err := s.findFleetClusterJoin(ctx, name); err == nil {
		http.Error(w, "FleetClusterJoin already exists", http.StatusConflict)
		return
	} else if !apierrors.IsNotFound(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	siteID := req.SiteId
	if siteID == "" {
		siteID = mcLabels(mc)["fleet.site"]
	}
	if siteID == "" {
		siteID = "homelab"
	}
	bootstrapHost := req.BootstrapHost
	if bootstrapHost == "" {
		bootstrapHost = adoptBootstrapHost(mc)
	}
	wgAddr := req.WireguardIP
	if wgAddr == "" {
		wgAddr = adoptWireguardCIDR(mc)
	}
	if wgAddr == "" {
		http.Error(w, "wireguardIp required (no fleet.topology/wireguard_ip on ManagedCluster)", http.StatusBadRequest)
		return
	}
	if !strings.Contains(wgAddr, "/") {
		wgAddr = wgAddr + "/32"
	}
	wgHost := wg.HostIP(wgAddr)

	install := fleetv1alpha1.ProviderInstallSpec{}
	ports, err := providers.AllocateNodePorts(ctx, s.Client, s.Cfg.Namespace, install)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if req.K8sNodePort > 0 {
		ports.K8s = req.K8sNodePort
	}
	if req.HelmNodePort > 0 {
		ports.Helm = req.HelmNodePort
	}
	if req.KubevirtPort > 0 {
		ports.Kubevirt = req.KubevirtPort
	}

	used, err := overlay.CollectUsed(ctx, s.Client, s.Cfg.Namespace, s.Cfg.ReservedOverlayHosts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := overlay.AssertAvailable(used, wgAddr); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	caps := registryCapabilitiesFromMC(mc)
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.Cfg.Namespace,
			Annotations: map[string]string{
				adoptAnnotation: "true",
			},
		},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName:         name,
			SiteId:              siteID,
			BootstrapHost:       bootstrapHost,
			Topology:            fleetv1alpha1.ClusterTopologySingleNode,
			NetworkMode:         fleetv1alpha1.NetworkModeWireguard,
			ClusterRegion:       mcLabels(mc)["fleet.topology/region"],
			ClusterCapabilities: caps,
			Nodes: []fleetv1alpha1.FleetClusterJoinNodeSpec{
				{Hostname: name, Address: bootstrapHost, Role: fleetv1alpha1.NodeRoleControl},
			},
			Providers: fleetv1alpha1.ClusterProvidersSpec{
				K8sNodePort:        ports.K8s,
				HelmNodePort:       ports.Helm,
				KubevirtSPNodeport: ports.Kubevirt,
			},
		},
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			Phase:              fleetv1alpha1.JoinPhaseReady,
			Message:            "Adopted inventory OCM cluster",
			WireguardAddress:   wgAddr,
			ManagedClusterAvailable: true,
			Providers: map[string]fleetv1alpha1.ClusterProviderStatus{
				fleetv1alpha1.ProviderKindK8s:      {Phase: fleetv1alpha1.ProviderPhaseNotInstalled},
				fleetv1alpha1.ProviderKindHelm:     {Phase: fleetv1alpha1.ProviderPhaseNotInstalled},
				fleetv1alpha1.ProviderKindKubevirt: {Phase: fleetv1alpha1.ProviderPhaseNotInstalled},
			},
		},
	}
	if err := s.Client.Create(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ini := buildAdoptedInventoryINI(s.Cfg, name, siteID, bootstrapHost, wgAddr, wgHost, caps["kvm"])
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterInventoryCMName(name),
			Namespace: s.Cfg.Namespace,
			Labels: map[string]string{
				"fleet.umkim.com/cluster": name,
				"fleet.umkim.com/adopted": "true",
			},
		},
		Data: map[string]string{"inventory.ini": ini},
	}
	var ex corev1.ConfigMap
	if err := s.Client.Get(ctx, client.ObjectKeyFromObject(cm), &ex); apierrors.IsNotFound(err) {
		if err := s.Client.Create(ctx, cm); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if err == nil {
		ex.Data = cm.Data
		if err := s.Client.Update(ctx, &ex); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.ensureAdoptSSHSecret(ctx, fcj, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ann := mc.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann["fleet.umkim.com/console-managed"] = "true"
	mc.SetAnnotations(ann)
	if err := s.Client.Update(ctx, mc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, clusterSummaryFromJoin(fcj))
}

func adoptBootstrapHost(mc *unstructured.Unstructured) string {
	ann := mc.GetAnnotations()
	if ann != nil {
		if v := ann["fleet.topology/bootstrap_host"]; v != "" {
			return v
		}
	}
	return ""
}

func adoptWireguardCIDR(mc *unstructured.Unstructured) string {
	labels := mcLabels(mc)
	if v := labels["fleet.topology/wireguard_ip"]; v != "" {
		return v
	}
	return ""
}

func registryCapabilitiesFromMC(mc *unstructured.Unstructured) map[string]bool {
	labels := mcLabels(mc)
	out := map[string]bool{}
	if labels["fleet.capability/gpu"] == "true" {
		out["gpu"] = true
	}
	if labels["fleet.capability/storage.rwo"] == "true" {
		out["storage.rwo"] = true
	}
	if labels["fleet.capability/storage.rwx"] == "true" {
		out["storage.rwx"] = true
	}
	if labels["fleet.capability/kvm"] == "true" {
		out["kvm"] = true
	}
	if labels["fleet.capability/kubevirt"] == "true" {
		out["kubevirt"] = true
	}
	if labels["fleet.capability/network.multus_l2"] == "true" {
		out["network.multus_l2"] = true
	}
	if labels["fleet.topology/ha"] == "true" {
		out["ha_topology"] = true
	}
	return out
}

func clusterInventoryCMName(clusterName string) string {
	return "fleet-cluster-" + clusterName + "-inventory"
}

func clusterAdoptSSHSecretName(clusterName string) string {
	return "fleet-cluster-" + clusterName + "-ansible-ssh"
}

func (s *Server) ensureAdoptSSHSecret(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) error {
	name := clusterAdoptSSHSecretName(clusterName)
	var ex corev1.Secret
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: fcj.Namespace, Name: name}, &ex); err == nil && len(ex.Data["id_rsa"]) > 0 {
		return nil
	}
	const shared = "fleet-site-ansible-ssh"
	var sharedSec corev1.Secret
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: fcj.Namespace, Name: shared}, &sharedSec); err != nil || len(sharedSec.Data["id_rsa"]) == 0 {
		return fmt.Errorf("missing Secret %s with id_rsa for provider jobs", shared)
	}
	key := append([]byte(nil), sharedSec.Data["id_rsa"]...)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fcj.Namespace,
			Labels: map[string]string{
				"fleet.umkim.com/cluster": clusterName,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{"id_rsa": key},
	}
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: fcj.Namespace, Name: name}, &ex); apierrors.IsNotFound(err) {
		return s.Client.Create(ctx, secret)
	} else if err != nil {
		return err
	}
	ex.Data = secret.Data
	return s.Client.Update(ctx, &ex)
}

func buildAdoptedInventoryINI(cfg config.Config, clusterName, siteID, bootstrapHost, wgCIDR, wgHost string, kvm bool) string {
	kubevirtEnable := "false"
	if kvm {
		kubevirtEnable = "true"
	}
	hubHost := cfg.HubHostInventoryName
	if hubHost == "" {
		hubHost = "umkim-hub"
	}
	sshUser := cfg.SSHUser
	if sshUser == "" {
		sshUser = "linux"
	}
	sshArgs := "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
	region := "homelab"
	return fmt.Sprintf(`[site_bootstrap]
%s ansible_host=%s ansible_user=%s

[site_bootstrap:vars]
ansible_ssh_common_args=%s
ansible_ssh_private_key_file=/root/.ssh/id_rsa
ocm_cluster_name=%s
ocm_hub_apiserver=https://%s:6443
ocm_hub_host=%s
ocm_hub=false
fleet_site_name=%s
fleet_cluster_name=%s
fleet_site_id=%s
fleet_site_wg_cidr=%s
fleet_site_wg_ip=%s
fleet_site_sp_endpoint=%s
wireguard_address=%s
fleet_site_enable_kubevirt=%s
fleet_cluster_kvm=%s
fleet_site_auto_install_kubevirt_operator=false
k3s_command_path=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
k3s_kubeconfig=/etc/rancher/k3s/k3s.yaml
ocm_kubeconfig=/etc/rancher/k3s/k3s.yaml
cluster_region=%s

[ocm_spoke_join:children]
site_bootstrap

[hub]
%s ansible_host=%s ansible_user=%s

[hub:vars]
wireguard_address=10.10.0.1/24
ocm_kubeconfig=/etc/rancher/k3s/k3s.yaml
k3s_command_path=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
`,
		clusterName, wgHost, sshUser,
		sshArgs,
		clusterName, cfg.HubWireguardIP, hubHost,
		clusterName, clusterName, siteID,
		wgCIDR, wgHost, wgHost, wgCIDR,
		kubevirtEnable, kubevirtEnable,
		region,
		hubHost, cfg.HubWireguardIP, sshUser,
	)
}
