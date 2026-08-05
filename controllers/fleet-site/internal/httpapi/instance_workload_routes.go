package httpapi

import (
	"context"

	fleetedge "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func listFleetAppRoutes(ctx context.Context, c client.Client, ns string) ([]appRouteSummary, error) {
	var list fleetedge.FleetAppRouteList
	if err := c.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return nil, err
	}
	out := make([]appRouteSummary, 0, len(list.Items))
	for i := range list.Items {
		route := &list.Items[i]
		id := route.Spec.InstanceID
		if id == "" && route.Labels != nil {
			id = route.Labels["edge.umkim.com/instance-id"]
		}
		out = append(out, appRouteSummary{
			instanceID:    id,
			hubHost:       route.Spec.HubHost,
			activeCluster: route.ActiveClusterName(),
		})
	}
	return out, nil
}
