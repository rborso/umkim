package reconcile

import (
	"context"

	fleetedge "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// deleteFleetAppRoutesForInstance removes FleetAppRoute CRs tied to a catalog instance ID.
func deleteFleetAppRoutesForInstance(ctx context.Context, c client.Client, namespace, instanceID string) error {
	if instanceID == "" || namespace == "" {
		return nil
	}
	var list fleetedge.FleetAppRouteList
	if err := c.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return err
	}
	for i := range list.Items {
		route := &list.Items[i]
		if route.Spec.InstanceID != instanceID && route.Labels["edge.umkim.com/instance-id"] != instanceID {
			continue
		}
		if err := c.Delete(ctx, route); err != nil {
			return err
		}
	}
	return nil
}
