package httpapi

import (
	"context"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var managedClusterGVK = schema.GroupVersionKind{
	Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
}

func listManagedClusters(ctx context.Context, c client.Client) ([]unstructured.Unstructured, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group: managedClusterGVK.Group, Version: managedClusterGVK.Version, Kind: managedClusterGVK.Kind + "List",
	})
	if err := c.List(ctx, list); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []unstructured.Unstructured
	for _, item := range list.Items {
		name := item.GetName()
		if name == "local-cluster" {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func getManagedCluster(ctx context.Context, c client.Client, name string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(managedClusterGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: name}, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func mcLabels(mc *unstructured.Unstructured) map[string]string {
	l := mc.GetLabels()
	if l == nil {
		return map[string]string{}
	}
	return l
}

func mcConditionAvailable(mc *unstructured.Unstructured) bool {
	conds, found, _ := unstructured.NestedSlice(mc.Object, "status", "conditions")
	if !found {
		return false
	}
	for _, c := range conds {
		m, _ := c.(map[string]interface{})
		if m == nil {
			continue
		}
		if m["type"] == "ManagedClusterConditionAvailable" && m["status"] == "True" {
			return true
		}
	}
	return false
}

func labelBool(labels map[string]string, key string) bool {
	return strings.EqualFold(labels[key], "true")
}
