package jobs

import (
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HubAnsibleSecretsName is the Secret with ansible_vars.yml (harbor_admin_password, etc.)
// for console-driven cluster provider install/delete jobs.
const HubAnsibleSecretsName = "fleet-site-hub-ansible-secrets"

func ansiblePlaybookArgs(playbook, clusterName, siteID string) []string {
	return []string{
		"ansible-playbook", playbook,
		"-e", "fleet_site_name=" + clusterName,
		"-e", "fleet_cluster_name=" + clusterName,
		"-e", "fleet_site_id=" + siteID,
		"-e", "@/ansible/inventory/group_vars/all/common.yml",
		"-e", "@/ansible/inventory/group_vars/all/site.yml",
		"-e", "@/ansible/inventory/group_vars/all/fleet_site.yml",
		"-i", "/inventory/inventory.ini",
	}
}

func ValidateJob(ns, name, clusterName, siteID, topology, runnerImage, sshSecret, inventoryCM string) *batchv1.Job {
	playbook := "/ansible/site-validate.yml"
	if topology == "ha_3_node" {
		playbook = "/ansible/cluster-validate.yml"
	}
	return ansibleJob(ns, "fleet-site-validate-"+name, clusterName, runnerImage, ansiblePlaybookArgs(playbook, clusterName, siteID), sshSecret, inventoryCM, "", 600)
}

func ProvisionJob(ns, name, clusterName, siteID, topology string, bundleMode bool, runnerImage, sshSecret, inventoryCM string) *batchv1.Job {
	playbook := "/ansible/site-provision.yml"
	deadline := int64(3600)
	if bundleMode {
		playbook = "/ansible/cluster-provision-hub.yml"
		deadline = 1800
		if topology == "ha_3_node" {
			// HA hub finalize (OCM join, CSR, ManifestWorks on 3 nodes) routinely exceeds 30m.
			deadline = 5400
		}
	} else if topology == "ha_3_node" {
		playbook = "/ansible/cluster-provision.yml"
		deadline = 5400
	}
	return ansibleJob(ns, "fleet-site-provision-"+name, clusterName, runnerImage, ansiblePlaybookArgs(playbook, clusterName, siteID), sshSecret, inventoryCM, "", deadline)
}

// WGPeer is a hub WireGuard peer to sync.
type WGPeer struct {
	PublicKey string
	AllowedIP string
}

const wgJobIfaceSetup = `set -e; apk add --no-cache wireguard-tools >/dev/null; IFACE=wg0;` +
	`if ! ip link show "$IFACE" 2>/dev/null; then IFACE=$(ip -o link show type wireguard 2>/dev/null | awk -F': ' '{print $2}' | head -1); fi;` +
	`test -n "$IFACE"; `

// Drop runtime peers with no AllowedIPs (abandoned enrollments that connected before hub sync).
const wgPruneStalePeersShell = `for _peer in $(wg show "$IFACE" dump | awk -F'\t' '$4=="(none)"{print $1}'); do ` +
	`[ -n "$_peer" ] && wg set "$IFACE" peer "$_peer" remove; ` +
	`done; `

func wgJobShell(add, remove []WGPeer) string {
	var b strings.Builder
	b.WriteString(wgJobIfaceSetup)
	b.WriteString(wgPruneStalePeersShell)
	for _, p := range remove {
		if p.PublicKey == "" {
			continue
		}
		b.WriteString(fmt.Sprintf(`wg set "$IFACE" peer %s remove; `, p.PublicKey))
	}
	for _, p := range add {
		if p.PublicKey == "" || p.AllowedIP == "" {
			continue
		}
		b.WriteString(fmt.Sprintf(`wg set "$IFACE" peer %s allowed-ips %s/32 persistent-keepalive 25; `, p.PublicKey, p.AllowedIP))
		b.WriteString(fmt.Sprintf(`ip route replace %s/32 dev "$IFACE"; `, p.AllowedIP))
	}
	b.WriteString(`wg show "$IFACE"; echo ok`)
	return b.String()
}

const wgJobTTLSecondsAfterFinished int32 = 300

func WgSyncJobMulti(ns, jobName string, peers []WGPeer, removePeers []WGPeer) *batchv1.Job {
	cmd := wgJobShell(peers, removePeers)
	ttl := wgJobTTLSecondsAfterFinished
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
			Labels:    map[string]string{"app": "fleet-site-wg-sync"},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "fleet-site-wg-sync"},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					HostNetwork:   true,
					NodeSelector:  map[string]string{"node-role.kubernetes.io/control-plane": "true"},
					Tolerations: []corev1.Toleration{
						{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
					},
					Containers: []corev1.Container{{
						Name:            "wg-sync",
						Image:           "docker.io/library/alpine:3.20",
						Command:         []string{"/bin/sh", "-c"},
						Args:            []string{cmd},
						SecurityContext: &corev1.SecurityContext{Privileged: ptrBool(true)},
					}},
				},
			},
		},
	}
}

func WgSyncJob(ns, jobName, pubKey, allowedIP string, removePeers []WGPeer) *batchv1.Job {
	return WgSyncJobMulti(ns, jobName, []WGPeer{{PublicKey: pubKey, AllowedIP: allowedIP}}, removePeers)
}

// OverlayConnectivityJob probes hub→spoke overlay reachability from the host network,
// re-applies the WireGuard peer + route when the handshake is stale, and verifies TCP to the SP NodePort.
func OverlayConnectivityJob(ns, jobName, pubKey, hostIP string, tcpPort int) *batchv1.Job {
	const staleSec = 90
	cmd := fmt.Sprintf(`set -e
apk add --no-cache wireguard-tools iputils iproute2 netcat-openbsd >/dev/null
IFACE=wg0
if ! ip link show "$IFACE" 2>/dev/null; then IFACE=$(ip -o link show type wireguard 2>/dev/null | awk -F': ' '{print $2}' | head -1); fi
test -n "$IFACE"
PUB=%q
HOST=%q
PORT=%d
NOW=$(date +%%s)
HS=$(wg show "$IFACE" latest-handshakes 2>/dev/null | awk -v p="$PUB" '$1==p{print $2; exit}')
if [ -z "$HS" ] || [ "$HS" = "0" ]; then AGE=999999; else AGE=$((NOW-HS)); fi
if ! ping -c1 -W2 "$HOST" >/dev/null 2>&1 || [ "$AGE" -gt %d ]; then
  wg set "$IFACE" peer "$PUB" allowed-ips "${HOST}/32" persistent-keepalive 25
  ip route replace "${HOST}/32" dev "$IFACE"
  sleep 3
fi
ping -c1 -W3 "$HOST" >/dev/null
if [ "$PORT" -gt 0 ]; then nc -z -w5 "$HOST" "$PORT"; fi
echo overlay_ok
`, pubKey, hostIP, tcpPort, staleSec)
	return hostNetworkPrivilegedJob(ns, jobName, "fleet-site-overlay-check", "overlay-check", cmd)
}

// OverlayRecoverJob touches provider registrations after overlay recovery.
func OverlayRecoverJob(ns, jobName, clusterName, siteID, runnerImage, sshSecret, inventoryCM string, extraArgs []string) *batchv1.Job {
	args := ansiblePlaybookArgs("/ansible/cluster-overlay-recover.yml", clusterName, siteID)
	args = append(args, "-e", "@/secrets/hub/ansible_vars.yml")
	args = append(args, extraArgs...)
	return ansibleJob(ns, jobName, clusterName, runnerImage, args, sshSecret, inventoryCM, HubAnsibleSecretsName, 300)
}

func hostNetworkPrivilegedJob(ns, jobName, app, containerName, cmd string) *batchv1.Job {
	ttl := int32(300)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
			Labels:    map[string]string{"app": app},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": app},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					HostNetwork:   true,
					NodeSelector:  map[string]string{"node-role.kubernetes.io/control-plane": "true"},
					Tolerations: []corev1.Toleration{
						{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
					},
					Containers: []corev1.Container{{
						Name:            containerName,
						Image:           "docker.io/library/alpine:3.20",
						Command:         []string{"/bin/sh", "-c"},
						Args:            []string{cmd},
						SecurityContext: &corev1.SecurityContext{Privileged: ptrBool(true)},
					}},
				},
			},
		},
	}
}

func WgRemoveJobMulti(ns, jobName string, peers []WGPeer) *batchv1.Job {
	var b strings.Builder
	b.WriteString(wgJobIfaceSetup)
	b.WriteString(wgPruneStalePeersShell)
	for _, p := range peers {
		if p.PublicKey == "" {
			continue
		}
		b.WriteString(fmt.Sprintf(`wg set "$IFACE" peer %s remove; `, p.PublicKey))
	}
	b.WriteString(`wg show "$IFACE"; echo ok`)
	cmd := b.String()
	ttl := wgJobTTLSecondsAfterFinished
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
			Labels:    map[string]string{"app": "fleet-site-wg-remove"},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "fleet-site-wg-remove"},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					HostNetwork:   true,
					NodeSelector:  map[string]string{"node-role.kubernetes.io/control-plane": "true"},
					Tolerations: []corev1.Toleration{
						{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
					},
					Containers: []corev1.Container{{
						Name:            "wg-remove",
						Image:           "docker.io/library/alpine:3.20",
						Command:         []string{"/bin/sh", "-c"},
						Args:            []string{cmd},
						SecurityContext: &corev1.SecurityContext{Privileged: ptrBool(true)},
					}},
				},
			},
		},
	}
}

func DiscoverJob(ns, jobName, clusterName, siteID, runnerImage, sshSecret, inventoryCM string) *batchv1.Job {
	args := ansiblePlaybookArgs("/ansible/cluster-discover-capabilities.yml", clusterName, siteID)
	return ansibleJob(ns, jobName, clusterName, runnerImage, args, sshSecret, inventoryCM, "", 600)
}

func KubeconfigSyncJob(ns, jobName, clusterName, siteID, runnerImage, sshSecret, inventoryCM string) *batchv1.Job {
	args := ansiblePlaybookArgs("/ansible/cluster-kubeconfig-sync.yml", clusterName, siteID)
	return ansibleJob(ns, jobName, clusterName, runnerImage, args, sshSecret, inventoryCM, "", 300)
}

func ConfigureStorageJob(ns, jobName, clusterName, siteID, runnerImage, sshSecret, inventoryCM string) *batchv1.Job {
	args := ansiblePlaybookArgs("/ansible/cluster-configure-storage.yml", clusterName, siteID)
	return ansibleJob(ns, jobName, clusterName, runnerImage, args, sshSecret, inventoryCM, "", 600)
}

func ConfigureKubevirtJob(ns, jobName, clusterName, siteID, runnerImage, sshSecret, inventoryCM string, extraArgs []string) *batchv1.Job {
	args := ansiblePlaybookArgs("/ansible/cluster-configure-kubevirt.yml", clusterName, siteID)
	args = append(args, "-e", "fleet_site_enable_kubevirt=true")
	args = append(args, "-e", "fleet_cluster_kvm=true")
	args = append(args, extraArgs...)
	return ansibleJob(ns, jobName, clusterName, runnerImage, args, sshSecret, inventoryCM, "", 5400)
}

func ValidateStorageJob(ns, jobName, clusterName, siteID, runnerImage, sshSecret, inventoryCM string, extraArgs []string) *batchv1.Job {
	args := ansiblePlaybookArgs("/ansible/cluster-validate-storage.yml", clusterName, siteID)
	args = append(args, extraArgs...)
	return ansibleJob(ns, jobName, clusterName, runnerImage, args, sshSecret, inventoryCM, "", 120)
}

func DeleteJob(ns, clusterName, siteID, runnerImage, sshSecret, inventoryCM string, hubOnly, cleanupKubernetes bool) *batchv1.Job {
	args := ansiblePlaybookArgs("/ansible/cluster-delete-hub.yml", clusterName, siteID)
	if hubOnly {
		args = append(args, "-e", "fleet_cluster_delete_hub_only=true")
	}
	if cleanupKubernetes {
		args = append(args, "-e", "fleet_cluster_cleanup_kubernetes=true")
	}
	return ansibleJob(ns, "fleet-cluster-delete-"+clusterName, clusterName, runnerImage, args, sshSecret, inventoryCM, "", 1200)
}

func nodeOperationJob(ns, jobName, clusterName, siteID, playbook, runnerImage, sshSecret, inventoryCM string, limit string, extraArgs []string, timeoutSec int64) *batchv1.Job {
	args := ansiblePlaybookArgs(playbook, clusterName, siteID)
	args = append(args, "-e", "@/secrets/hub/ansible_vars.yml")
	args = append(args, extraArgs...)
	if limit != "" {
		args = append(args, "--limit", limit)
	}
	j := ansibleJob(ns, jobName, clusterName, runnerImage, args, sshSecret, inventoryCM, HubAnsibleSecretsName, timeoutSec)
	ttl := int32(3600)
	j.Spec.TTLSecondsAfterFinished = &ttl
	if j.Labels == nil {
		j.Labels = map[string]string{}
	}
	j.Labels["fleet-cluster-nodeop"] = clusterName
	return j
}

func AddWorkerJob(ns, jobName, clusterName, siteID, targetHost, runnerImage, sshSecret, inventoryCM string, extraArgs []string) *batchv1.Job {
	limit := targetHost
	if limit != "" {
		limit += ",site_bootstrap"
	}
	return nodeOperationJob(ns, jobName, clusterName, siteID, "/ansible/cluster-add-worker.yml",
		runnerImage, sshSecret, inventoryCM, limit, extraArgs, 3600)
}

func RemoveWorkerJob(ns, jobName, clusterName, siteID, runnerImage, sshSecret, inventoryCM string, extraArgs []string) *batchv1.Job {
	return nodeOperationJob(ns, jobName, clusterName, siteID, "/ansible/cluster-remove-worker.yml",
		runnerImage, sshSecret, inventoryCM, "site_bootstrap", extraArgs, 2400)
}

func ReplaceControlJob(ns, jobName, clusterName, siteID, runnerImage, sshSecret, inventoryCM string, extraArgs []string) *batchv1.Job {
	return nodeOperationJob(ns, jobName, clusterName, siteID, "/ansible/cluster-replace-control.yml",
		runnerImage, sshSecret, inventoryCM, "", extraArgs, 5400)
}

// SiteProvisionConnectorJob runs site-provision-connector.yml against a jumphost inventory.
// gitSecret, when non-empty, mounts a deploy key at /etc/fleet/yaidap-git for sync_yaidap.
func SiteProvisionConnectorJob(ns, jobName, siteName, siteID, runnerImage, sshSecret, inventoryCM, gitSecret string, extraArgs []string) *batchv1.Job {
	args := []string{
		"ansible-playbook", "/ansible/site-provision-connector.yml",
		"-e", "fleet_site_name=" + siteName,
		"-e", "fleet_site_id=" + siteID,
		"-e", "@/ansible/inventory/group_vars/all/common.yml",
		"-i", "/inventory/inventory.ini",
		"--limit", "jumphost",
	}
	args = append(args, extraArgs...)
	j := ansibleJob(ns, jobName, siteName, runnerImage, args, sshSecret, inventoryCM, "", 5400)
	if strings.TrimSpace(gitSecret) != "" {
		j.Spec.Template.Spec.Containers[0].VolumeMounts = append(j.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "yaidap-git",
			MountPath: "/etc/fleet/yaidap-git",
			ReadOnly:  true,
		})
		j.Spec.Template.Spec.Volumes = append(j.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "yaidap-git",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  gitSecret,
					DefaultMode: ptrInt32(0600),
				},
			},
		})
	}
	return j
}

func IngressAuthReconcileJob(ns, jobName, runnerImage, inventoryCM, globalSecretsName string, enabledClusters []string) *batchv1.Job {
	args := []string{
		"ansible-playbook", "/ansible/ingress-auth-reconcile.yml",
		"-e", "@/inventory/ingress_auth_vars.yml",
		"-e", "@/ansible/inventory/group_vars/all/common.yml",
		"-i", "/inventory/inventory.ini",
	}
	return ingressAuthJob(ns, jobName, runnerImage, args, inventoryCM, globalSecretsName, enabledClusters, 3600)
}

func LLMReconcileJob(ns, jobName, runnerImage, inventoryCM string) *batchv1.Job {
	args := []string{
		"ansible-playbook", "/ansible/llm-reconcile.yml",
		"-e", "@/inventory/llm_vars.yml",
		"-e", "@/ansible/inventory/group_vars/all/common.yml",
		"-i", "/inventory/inventory.ini",
	}
	backoff := int32(1)
	deadline := int64(1800)
	ttl := int32(600)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
			Labels:    map[string]string{"app": "fleet-llm-ansible"},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			ActiveDeadlineSeconds:   &deadline,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "fleet-llm-ansible"},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					HostNetwork:        true,
					ServiceAccountName: "fleet-site-controller",
					Containers: []corev1.Container{{
						Name:            "ansible",
						Image:           runnerImage,
						ImagePullPolicy: corev1.PullAlways,
						Command:         args,
						VolumeMounts: []corev1.VolumeMount{
							{Name: "inventory", MountPath: "/inventory"},
							{Name: "kube", MountPath: "/etc/kube"},
						},
						Env: []corev1.EnvVar{
							{Name: "KUBECONFIG", Value: "/etc/kube/config"},
							{Name: "ANSIBLE_HOST_KEY_CHECKING", Value: "False"},
						},
					}},
					Volumes: []corev1.Volume{
						{Name: "inventory", VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: inventoryCM}},
						}},
						{Name: "kube", VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: "fleet-site-controller-kubeconfig"},
						}},
					},
				},
			},
		},
	}
}

func AcmeReconcileJob(ns, jobName, runnerImage, inventoryCM string) *batchv1.Job {
	args := []string{
		"ansible-playbook", "/ansible/acme-reconcile.yml",
		"-e", "@/inventory/acme_vars.yml",
		"-e", "@/ansible/inventory/group_vars/all/common.yml",
		"-e", "@/ansible/inventory/group_vars/all/site.yml",
		"-i", "/inventory/inventory.ini",
	}
	backoff := int32(1)
	deadline := int64(3600)
	ttl := int32(600)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
			Labels:    map[string]string{"app": "fleet-acme-ansible"},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			ActiveDeadlineSeconds:   &deadline,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "fleet-acme-ansible"},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					HostNetwork:        true,
					ServiceAccountName: "fleet-site-controller",
					Containers: []corev1.Container{{
						Name:            "ansible",
						Image:           runnerImage,
						ImagePullPolicy: corev1.PullAlways,
						Command:         args,
						VolumeMounts: []corev1.VolumeMount{
							{Name: "inventory", MountPath: "/inventory"},
							{Name: "kube", MountPath: "/etc/kube"},
						},
						Env: []corev1.EnvVar{
							{Name: "KUBECONFIG", Value: "/etc/kube/config"},
							{Name: "ANSIBLE_HOST_KEY_CHECKING", Value: "False"},
						},
					}},
					Volumes: []corev1.Volume{
						{Name: "inventory", VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: inventoryCM}},
						}},
						{Name: "kube", VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: "fleet-site-controller-kubeconfig"},
						}},
					},
				},
			},
		},
	}
}

func ingressAuthJob(ns, jobName, image string, cmd []string, inventoryCM, secretsName string, enabledClusters []string, timeoutSec int64) *batchv1.Job {
	backoff := int32(1)
	deadline := timeoutSec
	volumes := []corev1.Volume{
		{Name: "inventory", VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: inventoryCM}},
		}},
		{Name: "kube", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: "fleet-site-controller-kubeconfig"},
		}},
		{Name: "secrets", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: secretsName},
		}},
	}
	mounts := []corev1.VolumeMount{
		{Name: "inventory", MountPath: "/inventory"},
		{Name: "kube", MountPath: "/etc/kube"},
		{Name: "secrets", MountPath: "/secrets/ingress-auth", ReadOnly: true},
	}
	for _, cluster := range enabledClusters {
		volName := "cluster-secret-" + cluster
		volumes = append(volumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: "fleet-ingress-auth-" + cluster},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: "/secrets/ingress-auth/clusters/" + cluster,
			ReadOnly:  true,
		})
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
			Labels:    map[string]string{"app": "fleet-ingress-auth-ansible"},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			ActiveDeadlineSeconds:   &deadline,
			TTLSecondsAfterFinished: ptrInt32(86400),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "fleet-ingress-auth-ansible"},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					HostNetwork:        true,
					ServiceAccountName: "fleet-site-controller",
					Containers: []corev1.Container{{
						Name:            "ansible",
						Image:           image,
						ImagePullPolicy: corev1.PullAlways,
						Command:         cmd,
						VolumeMounts:    mounts,
						Env: []corev1.EnvVar{
							{Name: "KUBECONFIG", Value: "/etc/kube/config"},
							{Name: "ANSIBLE_HOST_KEY_CHECKING", Value: "False"},
						},
					}},
					Volumes: volumes,
				},
			},
		},
	}
}

func ansibleJob(ns, jobName, siteName, image string, cmd []string, sshSecret, inventoryCM, hubSecretsName string, timeoutSec int64) *batchv1.Job {
	backoff := int32(1)
	deadline := timeoutSec
	mounts := []corev1.VolumeMount{
		{Name: "inventory", MountPath: "/inventory"},
		{Name: "ssh", MountPath: "/root/.ssh", ReadOnly: true},
		{Name: "kube", MountPath: "/etc/kube"},
	}
	volumes := []corev1.Volume{
		{Name: "inventory", VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: inventoryCM}},
		}},
		{Name: "ssh", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  sshSecret,
				DefaultMode: ptrInt32(0600),
			},
		}},
		{Name: "kube", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: "fleet-site-controller-kubeconfig",
			},
		}},
	}
	if hubSecretsName != "" {
		mounts = append(mounts, corev1.VolumeMount{Name: "hub-secrets", MountPath: "/secrets/hub", ReadOnly: true})
		volumes = append(volumes, corev1.Volume{
			Name: "hub-secrets",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: hubSecretsName},
			},
		})
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
			Labels:    map[string]string{"app": "fleet-site-ansible", "fleet-site": siteName},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			ActiveDeadlineSeconds:   &deadline,
			TTLSecondsAfterFinished: ptrInt32(86400),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "fleet-site-ansible", "fleet-site": siteName},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					HostNetwork:        true,
					ServiceAccountName: "fleet-site-controller",
					Containers: []corev1.Container{{
						Name:            "ansible",
						Image:           image,
						ImagePullPolicy: corev1.PullAlways,
						Command:         cmd,
						VolumeMounts:    mounts,
						Env: []corev1.EnvVar{
							{Name: "KUBECONFIG", Value: "/etc/kube/config"},
							{Name: "ANSIBLE_HOST_KEY_CHECKING", Value: "False"},
						},
					}},
					Volumes: volumes,
				},
			},
		},
	}
}

func ptrBool(b bool) *bool    { return &b }
func ptrInt32(i int32) *int32 { return &i }

const helmAppJobScript = `set -euo pipefail
KC=/tmp/spoke.kubeconfig
kubectl -n fleet-system get secret "fleet-kubeconfig-${FLEET_APP_CLUSTER}" -o jsonpath='{.data.kubeconfig}' | base64 -d > "$KC"
test -s "$KC"
if [ "${FLEET_APP_ACTION}" = "uninstall" ]; then
  helm uninstall "${FLEET_APP_RELEASE}" --namespace "${FLEET_APP_NAMESPACE}" \
    --kubeconfig "$KC" --kube-insecure-skip-tls-verify --wait=false || true
  echo uninstalled
  exit 0
fi
if [ -n "${FLEET_APP_REGISTRY:-}" ]; then
  python3 - <<'PY'
import yaml
p = yaml.safe_load(open("/secrets/hub/ansible_vars.yml"))["harbor_admin_password"]
open("/tmp/harbor.pass", "w").write(str(p))
PY
  helm registry login "${FLEET_APP_REGISTRY}" -u "${FLEET_APP_REGISTRY_USER:-admin}" --password-stdin --insecure < /tmp/harbor.pass
  rm -f /tmp/harbor.pass
fi
VER=()
if [ -n "${FLEET_APP_VERSION:-}" ]; then
  VER=(--version "${FLEET_APP_VERSION}")
fi
VAL=()
if [ -s /values/values.yaml ]; then
  VAL=(-f /values/values.yaml)
fi
POST=()
if [ -s /imagemap/images.json ]; then
  cat > /tmp/fleet-image-rewrite.sh <<'EOS'
#!/bin/bash
python3 - <<'PY'
import json, sys
m = json.load(open("/imagemap/images.json"))
data = sys.stdin.read()
for src, dst in sorted(m.items(), key=lambda kv: -len(kv[0])):
    if src and dst:
        data = data.replace(src, dst)
sys.stdout.write(data)
PY
EOS
  chmod +x /tmp/fleet-image-rewrite.sh
  POST=(--post-renderer /tmp/fleet-image-rewrite.sh)
fi
helm upgrade --install "${FLEET_APP_RELEASE}" "${FLEET_APP_CHART}" "${VER[@]}" \
  --namespace "${FLEET_APP_NAMESPACE}" --create-namespace \
  --kubeconfig "$KC" --kube-insecure-skip-tls-verify --insecure-skip-tls-verify \
  "${VAL[@]}" "${POST[@]}"
echo installed
`

type HelmAppJobOpts struct {
	Namespace         string
	JobName           string
	Cluster           string
	Chart             string
	Version           string
	Release           string
	TargetNamespace   string
	Action            string
	RunnerImage       string
	ValuesConfigMap   string
	ImageMapConfigMap string
	RegistryHost      string
	HubSecretsName    string
}

func HelmAppJob(opts HelmAppJobOpts) *batchv1.Job {
	action := opts.Action
	if action == "" {
		action = "install"
	}
	backoff := int32(1)
	deadline := int64(900)
	ttl := int32(86400)
	env := []corev1.EnvVar{
		{Name: "KUBECONFIG", Value: "/etc/kube/config"},
		{Name: "FLEET_APP_ACTION", Value: action},
		{Name: "FLEET_APP_CLUSTER", Value: opts.Cluster},
		{Name: "FLEET_APP_CHART", Value: opts.Chart},
		{Name: "FLEET_APP_VERSION", Value: opts.Version},
		{Name: "FLEET_APP_RELEASE", Value: opts.Release},
		{Name: "FLEET_APP_NAMESPACE", Value: opts.TargetNamespace},
		{Name: "FLEET_APP_REGISTRY", Value: opts.RegistryHost},
		{Name: "FLEET_APP_REGISTRY_USER", Value: "admin"},
	}
	mounts := []corev1.VolumeMount{
		{Name: "kube", MountPath: "/etc/kube", ReadOnly: true},
	}
	volumes := []corev1.Volume{
		{Name: "kube", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: "fleet-site-controller-kubeconfig"},
		}},
	}
	if opts.HubSecretsName != "" {
		mounts = append(mounts, corev1.VolumeMount{Name: "hub-secrets", MountPath: "/secrets/hub", ReadOnly: true})
		volumes = append(volumes, corev1.Volume{
			Name: "hub-secrets",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: opts.HubSecretsName},
			},
		})
	}
	if opts.ValuesConfigMap != "" && action != "uninstall" {
		mounts = append(mounts, corev1.VolumeMount{Name: "values", MountPath: "/values", ReadOnly: true})
		volumes = append(volumes, corev1.Volume{
			Name: "values",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: opts.ValuesConfigMap}},
			},
		})
	}
	if opts.ImageMapConfigMap != "" && action != "uninstall" {
		mounts = append(mounts, corev1.VolumeMount{Name: "imagemap", MountPath: "/imagemap", ReadOnly: true})
		volumes = append(volumes, corev1.Volume{
			Name: "imagemap",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: opts.ImageMapConfigMap}},
			},
		})
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.JobName,
			Namespace: opts.Namespace,
			Labels: map[string]string{
				"app":           "fleet-app-helm",
				"fleet-cluster": opts.Cluster,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			ActiveDeadlineSeconds:   &deadline,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "fleet-app-helm", "fleet-cluster": opts.Cluster},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					HostNetwork:        true,
					ServiceAccountName: "fleet-site-controller",
					Containers: []corev1.Container{{
						Name:            "helm",
						Image:           opts.RunnerImage,
						ImagePullPolicy: corev1.PullAlways,
						Command:         []string{"/bin/bash", "-c", helmAppJobScript},
						Env:             env,
						VolumeMounts:    mounts,
					}},
					Volumes: volumes,
				},
			},
		},
	}
}

type ChartPublishJobOpts struct {
	Namespace      string
	JobName        string
	ChartName      string
	Version        string
	Source         string
	Origin         string
	InboxFile      string
	InboxHostPath  string
	ImagePolicy    string
	ValuesYAML     string
	DestOCI        string
	MirrorHost     string
	RunnerImage    string
	HubSecretsName string
	ResultCM       string
}

const chartPublishJobScript = `set -euo pipefail
python3 - <<'PY'
import yaml
p = yaml.safe_load(open("/secrets/hub/ansible_vars.yml"))["harbor_admin_password"]
open("/tmp/harbor.pass", "w").write(str(p))
PY
DEST="${FLEET_CHART_DEST}"
DEST_HOST="${DEST#oci://}"
DEST_HOST="${DEST_HOST%%/*}"
helm registry login "${DEST_HOST}" -u admin --password-stdin --insecure < /tmp/harbor.pass
WORKDIR=/tmp/chart-work
mkdir -p "$WORKDIR"
cd "$WORKDIR"
SOURCE="${FLEET_CHART_SOURCE}"
ORIGIN="${FLEET_CHART_ORIGIN:-}"
VERSION="${FLEET_CHART_VERSION:-}"
TGZ=""
if [ "$SOURCE" = "upload" ]; then
  TGZ="/inbox/${FLEET_CHART_INBOX_FILE}"
  test -s "$TGZ"
elif [ "$SOURCE" = "oci" ]; then
  PULL=(helm pull "$ORIGIN" --destination . --insecure-skip-tls-verify)
  if [ -n "$VERSION" ]; then PULL+=(--version "$VERSION"); fi
  "${PULL[@]}"
  TGZ=$(ls -1 *.tgz | head -1)
elif [ "$SOURCE" = "http" ]; then
  curl -fsSL -o chart.tgz "$ORIGIN"
  TGZ=chart.tgz
elif [ "$SOURCE" = "harbor" ]; then
  echo "harbor import; no push"
else
  echo "unknown source $SOURCE" >&2
  exit 1
fi
if [ -n "$TGZ" ]; then
  META=$(helm show chart "$TGZ")
  NAME=$(printf '%s\n' "$META" | awk '/^name:/{print $2; exit}')
  VER=$(printf '%s\n' "$META" | awk '/^version:/{print $2; exit}')
  [ -n "${FLEET_CHART_NAME:-}" ] && NAME="${FLEET_CHART_NAME}"
  [ -n "$VERSION" ] && VER="$VERSION"
  PUSH_OUT=$(helm push "$TGZ" "$DEST" --insecure-skip-tls-verify 2>&1)
  printf '%s\n' "$PUSH_OUT"
  DIGEST=$(printf '%s\n' "$PUSH_OUT" | awk '/^Digest:/{print $2; exit}')
else
  NAME="${FLEET_CHART_NAME}"
  VER="${VERSION}"
  DIGEST=""
fi
CHART="${DEST%/}/${NAME}"
COPIED_JSON='[]'
IMAGE_VALUES=""
POLICY="${FLEET_CHART_IMAGE_POLICY:-external}"
if [ "$POLICY" = "copy" ] && [ -n "$TGZ" ]; then
  mkdir -p /tmp/chart-src
  tar -xzf "$TGZ" -C /tmp/chart-src
  CHARTDIR=$(find /tmp/chart-src -mindepth 1 -maxdepth 1 -type d | head -1)
  VALF=/tmp/pub-values.yaml
  if [ -n "${FLEET_CHART_VALUES:-}" ]; then
    printf '%s\n' "${FLEET_CHART_VALUES}" > "$VALF"
  else
    echo '{}' > "$VALF"
  fi
  helm template preview "$CHARTDIR" -f "$VALF" > /tmp/rendered.yaml
  python3 - <<'PY'
import json, os, re, subprocess, sys
rendered = open("/tmp/rendered.yaml").read()
imgs = []
for m in re.finditer(r"image:\s+(\S+)", rendered):
    imgs.append(m.group(1).strip().strip("\"'"))
    imgs.append(m.group(1).strip())
uniq = []
seen = set()
for i in imgs:
    if i and i not in seen:
        seen.add(i)
        uniq.append(i)

def normalize(ref):
    if ref.startswith("sha256:"):
        return ""
    host = ref.split("/")[0]
    if "/" not in ref or ("." not in host and ":" not in host and host != "localhost"):
        if "/" not in ref:
            ref = "docker.io/library/" + ref
        else:
            ref = "docker.io/" + ref
    return ref

mirror = os.environ.get("FLEET_CHART_MIRROR", "10.10.0.1:30443/mirror")
pw = open("/tmp/harbor.pass").read().strip()
copied = []
for raw in uniq:
    src = normalize(raw)
    if not src:
        continue
    dest = mirror.rstrip("/") + "/" + src
    cmd = [
        "skopeo", "copy", "--src-tls-verify=false", "--dest-tls-verify=false",
        "--dest-creds", "admin:" + pw,
        "docker://" + src, "docker://" + dest,
    ]
    print("copy", src, "->", dest, flush=True)
    r = subprocess.run(cmd, capture_output=True, text=True)
    if r.returncode != 0:
        sys.stderr.write(r.stdout + r.stderr)
        sys.stderr.write("failed to copy image %s\n" % src)
        sys.exit(1)
    copied.append({"from": src, "to": dest})
open("/tmp/copied.json", "w").write(json.dumps(copied))
lines = ["fleet:", "  copiedImages:"]
for c in copied:
    lines.append("    - from: %s" % json.dumps(c["from"]))
    lines.append("      to: %s" % json.dumps(c["to"]))
open("/tmp/image-values.yaml", "w").write("\n".join(lines) + "\n")
print("copied", len(copied), "images", flush=True)
PY
  COPIED_JSON=$(cat /tmp/copied.json)
  IMAGE_VALUES=$(cat /tmp/image-values.yaml)
elif [ "$POLICY" = "copy" ]; then
  echo "imagePolicy copy requires a packaged chart" >&2
  exit 1
fi
printf '%s\n' "$COPIED_JSON" > /tmp/copied.json
printf '%s\n' "$IMAGE_VALUES" > /tmp/image-values.yaml
kubectl -n "${FLEET_CHART_NAMESPACE}" create configmap "${FLEET_CHART_RESULT_CM}" \
  --from-literal=name="${NAME}" \
  --from-literal=version="${VER}" \
  --from-literal=digest="${DIGEST}" \
  --from-literal=chart="${CHART}" \
  --from-file=imagesCopied.json=/tmp/copied.json \
  --from-file=imageValues.yaml=/tmp/image-values.yaml \
  --dry-run=client -o yaml | kubectl apply -f -
echo published "${CHART}" "${VER}"
`

func ChartPublishJob(opts ChartPublishJobOpts) *batchv1.Job {
	backoff := int32(1)
	deadline := int64(1800)
	ttl := int32(86400)
	inboxPath := opts.InboxHostPath
	if inboxPath == "" {
		inboxPath = "/var/lib/umkim/chart-inbox"
	}
	env := []corev1.EnvVar{
		{Name: "KUBECONFIG", Value: "/etc/kube/config"},
		{Name: "FLEET_CHART_NAME", Value: opts.ChartName},
		{Name: "FLEET_CHART_VERSION", Value: opts.Version},
		{Name: "FLEET_CHART_SOURCE", Value: opts.Source},
		{Name: "FLEET_CHART_ORIGIN", Value: opts.Origin},
		{Name: "FLEET_CHART_INBOX_FILE", Value: opts.InboxFile},
		{Name: "FLEET_CHART_IMAGE_POLICY", Value: opts.ImagePolicy},
		{Name: "FLEET_CHART_VALUES", Value: opts.ValuesYAML},
		{Name: "FLEET_CHART_DEST", Value: opts.DestOCI},
		{Name: "FLEET_CHART_MIRROR", Value: opts.MirrorHost},
		{Name: "FLEET_CHART_NAMESPACE", Value: opts.Namespace},
		{Name: "FLEET_CHART_RESULT_CM", Value: opts.ResultCM},
	}
	mounts := []corev1.VolumeMount{
		{Name: "kube", MountPath: "/etc/kube", ReadOnly: true},
		{Name: "hub-secrets", MountPath: "/secrets/hub", ReadOnly: true},
	}
	volumes := []corev1.Volume{
		{Name: "kube", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: "fleet-site-controller-kubeconfig"},
		}},
		{Name: "hub-secrets", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: opts.HubSecretsName},
		}},
	}
	if opts.Source == "upload" && opts.InboxFile != "" {
		dirOrCreate := corev1.HostPathDirectoryOrCreate
		mounts = append(mounts, corev1.VolumeMount{Name: "inbox", MountPath: "/inbox"})
		volumes = append(volumes, corev1.Volume{
			Name: "inbox",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{Path: inboxPath, Type: &dirOrCreate},
			},
		})
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.JobName,
			Namespace: opts.Namespace,
			Labels:    map[string]string{"app": "fleet-chart-publish", "fleet-chart": opts.ChartName},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			ActiveDeadlineSeconds:   &deadline,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "fleet-chart-publish", "fleet-chart": opts.ChartName},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					HostNetwork:        true,
					ServiceAccountName: "fleet-site-controller",
					Containers: []corev1.Container{{
						Name:            "publish",
						Image:           opts.RunnerImage,
						ImagePullPolicy: corev1.PullAlways,
						Command:         []string{"/bin/bash", "-c", chartPublishJobScript},
						Env:             env,
						VolumeMounts:    mounts,
					}},
					Volumes: volumes,
				},
			},
		},
	}
}
