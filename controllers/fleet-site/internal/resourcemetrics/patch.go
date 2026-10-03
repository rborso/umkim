package resourcemetrics

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchManagedClusterLabels merges resource metric labels onto a ManagedCluster without removing unrelated keys.
func PatchManagedClusterLabels(ctx context.Context, c client.Client, clusterName string, labels map[string]string) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(managedClusterGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: clusterName}, obj); err != nil {
		return err
	}
	merged := obj.GetLabels()
	if merged == nil {
		merged = map[string]string{}
	}
	for k, v := range labels {
		if v == "" {
			delete(merged, k)
		} else {
			merged[k] = v
		}
	}
	patch := client.MergeFrom(obj.DeepCopy())
	obj.SetLabels(merged)
	return c.Patch(ctx, obj, patch)
}

// MergeResourceLabels builds labels for a scrape result, clearing stale utilization when metrics are unavailable.
func MergeResourceLabels(u Utilization) map[string]string {
	labels := LabelsForUtilization(u)
	if !u.MetricsOK {
		labels[LabelCPUUtilization] = ""
		labels[LabelMemoryUtilization] = ""
	}
	return labels
}

// ApplyLabelsToCluster scrapes utilization and patches ManagedCluster labels.
func ApplyLabelsToCluster(ctx context.Context, scraper *Scraper, hubClient client.Client, clusterName string) error {
	u, err := scraper.ScrapeCluster(ctx, clusterName)
	if err != nil {
		fail := Utilization{MetricsOK: false, MetricsAt: time.Now().UTC()}
		return PatchManagedClusterLabels(ctx, hubClient, clusterName, MergeResourceLabels(fail))
	}
	return PatchManagedClusterLabels(ctx, hubClient, clusterName, MergeResourceLabels(u))
}
