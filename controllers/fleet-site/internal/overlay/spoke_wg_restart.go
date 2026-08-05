package overlay

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const wgIface = "wg-umkim"

// SpokeWGRestartManifestWork returns an OCM ManifestWork that restarts wg-umkim on the spoke
// via a privileged hostPID Job (klusterlet applies it when overlay SSH is down).
func SpokeWGRestartManifestWork(clusterName, workName string) *unstructured.Unstructured {
	jobName := workName
	script := fmt.Sprintf(`set -e
IFACE=%s
nsenter -t 1 -m -u -i -n -p -- systemctl restart wg-quick@${IFACE} 2>/dev/null || \
(nsenter -t 1 -m -u -i -n -p -- wg-quick down ${IFACE} 2>/dev/null || true; \
 nsenter -t 1 -m -u -i -n -p -- wg-quick up ${IFACE})
nsenter -t 1 -m -u -i -n -p -- wg show ${IFACE}
echo ok
`, wgIface)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      workName,
				"namespace": clusterName,
				"labels": map[string]interface{}{
					"fleet.umkim.com/overlay-recover": "true",
				},
			},
			"spec": map[string]interface{}{
				"deleteOption": map[string]interface{}{
					"propagationPolicy": "Foreground",
				},
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "batch/v1",
							"kind":       "Job",
							"metadata": map[string]interface{}{
								"name":      jobName,
								"namespace": "kube-system",
								"labels": map[string]interface{}{
									"fleet.umkim.com/overlay-recover": "true",
								},
							},
							"spec": map[string]interface{}{
								"ttlSecondsAfterFinished": int64(300),
								"backoffLimit":            int64(1),
								"template": map[string]interface{}{
									"metadata": map[string]interface{}{
										"labels": map[string]interface{}{
											"fleet.umkim.com/overlay-recover": "true",
										},
									},
									"spec": map[string]interface{}{
										"hostNetwork":   true,
										"hostPID":       true,
										"restartPolicy": "Never",
										"nodeSelector": map[string]interface{}{
											"kubernetes.io/os": "linux",
										},
										"tolerations": []interface{}{
											map[string]interface{}{"operator": "Exists"},
										},
										"containers": []interface{}{
											map[string]interface{}{
												"name":  "wg-restart",
												"image": "docker.io/library/alpine:3.20",
												"securityContext": map[string]interface{}{
													"privileged": true,
												},
												"command": []interface{}{"/bin/sh", "-c"},
												"args":    []interface{}{script},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// SpokeWGRestartWorkName builds a unique ManifestWork name for overlay recovery.
func SpokeWGRestartWorkName(clusterName string) string {
	return fmt.Sprintf("fleet-overlay-wg-restart-%s-%d", clusterName, time.Now().Unix())
}

// SpokeWGRestartManifestWorkGVK is the GVK for ManifestWork resources.
var SpokeWGRestartManifestWorkGVK = metav1.GroupVersionKind{
	Group: "work.open-cluster-management.io", Version: "v1", Kind: "ManifestWork",
}
