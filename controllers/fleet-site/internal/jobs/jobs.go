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

func WgSyncJobMulti(ns, jobName string, peers []WGPeer, removePeers []WGPeer) *batchv1.Job {
	cmd := wgJobShell(peers, removePeers)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
			Labels:    map[string]string{"app": "fleet-site-wg-sync"},
		},
		Spec: batchv1.JobSpec{
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

// OverlayRecoverJob touches DCM SPM provider registrations after overlay recovery.
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
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
			Labels:    map[string]string{"app": "fleet-site-wg-remove"},
		},
		Spec: batchv1.JobSpec{
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

func providerJobDeadlineSec(kind, action string) int64 {
	if action == "delete" {
		return 1200
	}
	// kubevirt_stack (Multus, CDI, operator waits) routinely exceeds 30m on first install.
	if kind == "kubevirt" {
		return 5400
	}
	return 1800
}

func ProviderJob(ns, jobName, clusterName, siteID, kind, action, runnerImage, sshSecret, inventoryCM string, extraArgs []string) *batchv1.Job {
	playbook := fmt.Sprintf("/ansible/cluster-provider-%s-%s.yml", kind, action)
	args := ansiblePlaybookArgs(playbook, clusterName, siteID)
	args = append(args, "-e", "@/secrets/hub/ansible_vars.yml")
	args = append(args, extraArgs...)
	return ansibleJob(ns, jobName, clusterName, runnerImage, args, sshSecret, inventoryCM, HubAnsibleSecretsName, providerJobDeadlineSec(kind, action))
}

func ConfigureStorageJob(ns, jobName, clusterName, siteID, runnerImage, sshSecret, inventoryCM string) *batchv1.Job {
	args := ansiblePlaybookArgs("/ansible/cluster-configure-storage.yml", clusterName, siteID)
	return ansibleJob(ns, jobName, clusterName, runnerImage, args, sshSecret, inventoryCM, "", 600)
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

func ptrBool(b bool) *bool       { return &b }
func ptrInt32(i int32) *int32    { return &i }

const InstanceBackupInventoryCM = "fleet-instance-backup-inventory"

// InstanceBackupJob runs Velero backup + DCM export on the hub.
func InstanceBackupJob(ns, jobName, runnerImage, inventoryCM string, extraArgs []string) *batchv1.Job {
	args := []string{
		"ansible-playbook", "/ansible/instance-backup.yml",
		"-e", "@/ansible/inventory/group_vars/all/common.yml",
		"-e", "@/secrets/hub/ansible_vars.yml",
		"-i", "/inventory/inventory.ini",
	}
	args = append(args, extraArgs...)
	return hubKubeJob(ns, jobName, "fleet-instance-backup", runnerImage, inventoryCM, args, 1800)
}

// InstanceRestoreJob runs Velero restore on a spoke via hub kubeconfig.
func InstanceRestoreJob(ns, jobName, runnerImage, inventoryCM string, extraArgs []string) *batchv1.Job {
	args := []string{
		"ansible-playbook", "/ansible/instance-restore.yml",
		"-e", "@/ansible/inventory/group_vars/all/common.yml",
		"-e", "@/secrets/hub/ansible_vars.yml",
		"-i", "/inventory/inventory.ini",
	}
	args = append(args, extraArgs...)
	return hubKubeJob(ns, jobName, "fleet-instance-restore", runnerImage, inventoryCM, args, 1800)
}

func hubKubeJob(ns, jobName, app, runnerImage, inventoryCM string, cmd []string, timeoutSec int64) *batchv1.Job {
	backoff := int32(1)
	deadline := timeoutSec
	ttl := int32(300)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
			Labels:    map[string]string{"app": app},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			ActiveDeadlineSeconds:   &deadline,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": app},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					HostNetwork:        true,
					ServiceAccountName: "fleet-site-controller",
					Containers: []corev1.Container{{
						Name:            "ansible",
						Image:           runnerImage,
						ImagePullPolicy: corev1.PullAlways,
						Command:         cmd,
						VolumeMounts: []corev1.VolumeMount{
							{Name: "inventory", MountPath: "/inventory"},
							{Name: "kube", MountPath: "/etc/kube"},
							{Name: "hub-secrets", MountPath: "/secrets/hub", ReadOnly: true},
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
						{Name: "hub-secrets", VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: HubAnsibleSecretsName},
						}},
					},
				},
			},
		},
	}
}
