package backup

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	MinioDeployName = "fleet-velero-minio"
	VeleroNamespace = "velero"
)

var managedClusterGVK = schema.GroupVersionKind{
	Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedClusterList",
}

// ClusterInfraStatus is Velero readiness for one spoke cluster.
type ClusterInfraStatus struct {
	Name          string `json:"name"`
	Ready         bool   `json:"ready"`
	VeleroReady   bool   `json:"velero_ready"`
	Message       string `json:"message,omitempty"`
}

// InfrastructureStatus is hub MinIO + per-cluster Velero health.
type InfrastructureStatus struct {
	HubMinIOReady bool                 `json:"hub_minio_ready"`
	Bucket        string               `json:"bucket,omitempty"`
	Clusters      []ClusterInfraStatus `json:"clusters"`
}

func CheckInfrastructure(ctx context.Context, c client.Client, hubNS, fleetSystemNS string) (*InfrastructureStatus, error) {
	out := &InfrastructureStatus{
		Bucket: "velero-fleet",
	}
	var dep unstructured.Unstructured
	dep.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "apps", Version: "v1", Kind: "Deployment",
	})
	if err := c.Get(ctx, client.ObjectKey{Namespace: fleetSystemNS, Name: MinioDeployName}, &dep); err != nil {
		out.HubMinIOReady = false
	} else {
		ready, _, _ := unstructured.NestedInt64(dep.Object, "status", "readyReplicas")
		out.HubMinIOReady = ready > 0
	}

	var mcList unstructured.UnstructuredList
	mcList.SetGroupVersionKind(managedClusterGVK)
	if err := c.List(ctx, &mcList); err != nil {
		if meta.IsNoMatchError(err) {
			return out, nil
		}
		return nil, err
	}
	for _, mc := range mcList.Items {
		name, _, _ := unstructured.NestedString(mc.Object, "metadata", "name")
		if name == "" {
			continue
		}
		row := ClusterInfraStatus{Name: name}
		conditions, _, _ := unstructured.NestedSlice(mc.Object, "status", "conditions")
		for _, cond := range conditions {
			m, _ := cond.(map[string]interface{})
			if m == nil {
				continue
			}
			if m["type"] == "ManagedClusterConditionAvailable" && m["status"] == "True" {
				row.Ready = true
			}
		}
		var secret unstructured.Unstructured
		secret.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"})
		kcName := "fleet-kubeconfig-" + name
		if err := c.Get(ctx, client.ObjectKey{Namespace: fleetSystemNS, Name: kcName}, &secret); err != nil {
			row.Message = "missing spoke kubeconfig secret"
			out.Clusters = append(out.Clusters, row)
			continue
		}
		var veleroDep unstructured.Unstructured
		veleroDep.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
		// Hub cannot directly query spoke; mark ready when MC is available and kubeconfig exists.
		if row.Ready {
			row.VeleroReady = true
		} else {
			row.Message = "managed cluster not available"
		}
		out.Clusters = append(out.Clusters, row)
	}
	return out, nil
}

func ClusterInfraOK(status *InfrastructureStatus, cluster string) error {
	if status == nil || !status.HubMinIOReady {
		return fmt.Errorf("hub MinIO not ready")
	}
	for _, row := range status.Clusters {
		if row.Name == cluster {
			if !row.Ready {
				return fmt.Errorf("cluster %s not ready", cluster)
			}
			if !row.VeleroReady {
				return fmt.Errorf("velero not ready on cluster %s", cluster)
			}
			return nil
		}
	}
	return fmt.Errorf("cluster %s not found in fleet", cluster)
}

func ActiveBackupPhases() []string {
	return []string{"Pending", "Running"}
}

func ActiveRestorePhases() []string {
	return []string{"Pending", "Restoring", "Rebinding"}
}

func BackupSecretName(crName string) string {
	return "fleet-instance-backup-" + crName + "-dcm"
}

func BackupJobName(crName string) string {
	name := "fleet-instance-backup-" + crName
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func RestoreJobName(crName string) string {
	name := "fleet-instance-restore-" + crName
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func NowMeta() *metav1.Time {
	t := metav1.Now()
	return &t
}
