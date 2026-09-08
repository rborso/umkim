package nodeops

import (
	"context"
	"encoding/json"
	"fmt"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const AnnotationNodeOperationRequest = "fleet.umkim.com/node-operation-request"

// SubmitRequest queues a node operation on a Ready FleetClusterJoin.
func SubmitRequest(ctx context.Context, c client.Client, ns, clusterName string, req *Request) error {
	if req == nil || req.Type == "" {
		return fmt.Errorf("node operation type required")
	}
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: clusterName}, &fcj); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("cluster not found")
		}
		return err
	}
	if fcj.Spec.ClusterName != "" && fcj.Spec.ClusterName != clusterName {
		// allow lookup by metadata.name
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		return fmt.Errorf("cluster is not ready")
	}
	if fcj.Status.NodeOperation != nil &&
		(fcj.Status.NodeOperation.Phase == fleetv1alpha1.NodeOpPhaseRunning ||
			fcj.Status.NodeOperation.Phase == fleetv1alpha1.NodeOpPhasePending) {
		return fmt.Errorf("cluster node operation already running")
	}
	if fcj.Status.Storage.Phase == fleetv1alpha1.StoragePhasePending ||
		fcj.Status.Storage.Phase == fleetv1alpha1.StoragePhaseApplying {
		return fmt.Errorf("cluster storage operation already running")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	original := fcj.DeepCopy()
	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	fcj.Annotations[AnnotationNodeOperationRequest] = string(raw)
	return c.Patch(ctx, &fcj, client.MergeFrom(original))
}

// FindFleetClusterJoin resolves by metadata.name or spec.clusterName.
func FindFleetClusterJoin(ctx context.Context, c client.Client, ns, clusterName string) (*fleetv1alpha1.FleetClusterJoin, error) {
	var fcj fleetv1alpha1.FleetClusterJoin
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: clusterName}, &fcj); err == nil {
		return &fcj, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}
	list := &fleetv1alpha1.FleetClusterJoinList{}
	if err := c.List(ctx, list, client.InNamespace(ns)); err != nil {
		return nil, err
	}
	for i := range list.Items {
		if list.Items[i].Spec.ClusterName == clusterName {
			return &list.Items[i], nil
		}
	}
	return nil, fmt.Errorf("cluster %q not found", clusterName)
}
