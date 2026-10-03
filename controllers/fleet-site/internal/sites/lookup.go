package sites

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FindFleetSite resolves id as metadata.name, spec.siteId, or spec.siteName.
func FindFleetSite(ctx context.Context, c client.Client, namespace, id string) (*fleetv1alpha1.FleetSite, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("site not found")
	}
	var fs fleetv1alpha1.FleetSite
	err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: id}, &fs)
	if err == nil {
		return &fs, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}
	var list fleetv1alpha1.FleetSiteList
	if err := c.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	for i := range list.Items {
		item := &list.Items[i]
		if item.Spec.SiteId == id || item.Spec.SiteName == id {
			return item, nil
		}
	}
	return nil, fmt.Errorf("site not found")
}
