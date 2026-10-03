package reconcile

import (
	"context"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// reconcileClusterProviders is intentionally a no-op: v0.2 removed legacy
// k8s/helm/kubevirt service-provider Jobs. Ready clusters install Helm with
// FleetApp; KubeVirt uses reconcileClusterKubevirt / platform API.
func (r *ClusterJoinReconciler) reconcileClusterProviders(ctx context.Context, fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func allProviderKinds() []string {
	return []string{
		fleetv1alpha1.ProviderKindKubevirt,
		fleetv1alpha1.ProviderKindK8s,
		fleetv1alpha1.ProviderKindHelm,
	}
}

func providerNameForKind(fcj *fleetv1alpha1.FleetClusterJoin, clusterName, kind string) string {
	switch kind {
	case fleetv1alpha1.ProviderKindHelm:
		return fcj.Spec.HelmProviderName(clusterName)
	case fleetv1alpha1.ProviderKindKubevirt:
		return fcj.Spec.KubevirtProviderName(clusterName)
	default:
		return fcj.Spec.K8sProviderName(clusterName)
	}
}

func ensureProviderStatusDefaults(fcj *fleetv1alpha1.FleetClusterJoin, clusterName string) {
	if fcj.Status.Providers == nil {
		fcj.Status.Providers = map[string]fleetv1alpha1.ClusterProviderStatus{}
	}
	for _, kind := range allProviderKinds() {
		if _, ok := fcj.Status.Providers[kind]; !ok {
			fcj.Status.Providers[kind] = fleetv1alpha1.ClusterProviderStatus{
				Phase:        fleetv1alpha1.ProviderPhaseNotInstalled,
				ProviderName: providerNameForKind(fcj, clusterName, kind),
			}
		}
	}
}
