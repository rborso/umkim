package ingressauth

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LegacyStackDeployed reports whether the Ansible-installed hub auth stack is present.
func LegacyStackDeployed(ctx context.Context, c client.Client) bool {
	var dep appsv1.Deployment
	err := c.Get(ctx, types.NamespacedName{Namespace: "fleet-ingress-auth", Name: "authelia"}, &dep)
	return err == nil
}

func LegacyStackDeployedSafe(ctx context.Context, c client.Client) (bool, error) {
	var dep appsv1.Deployment
	err := c.Get(ctx, types.NamespacedName{Namespace: "fleet-ingress-auth", Name: "authelia"}, &dep)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}
