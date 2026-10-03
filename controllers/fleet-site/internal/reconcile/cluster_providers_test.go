package reconcile

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestProviderInstallOrder(t *testing.T) {
	want := []string{
		fleetv1alpha1.ProviderKindKubevirt,
		fleetv1alpha1.ProviderKindK8s,
		fleetv1alpha1.ProviderKindHelm,
	}
	if len(fleetv1alpha1.ProviderInstallOrder) != len(want) {
		t.Fatalf("order len %d want %d", len(fleetv1alpha1.ProviderInstallOrder), len(want))
	}
	for i := range want {
		if fleetv1alpha1.ProviderInstallOrder[i] != want[i] {
			t.Fatalf("index %d: got %s want %s", i, fleetv1alpha1.ProviderInstallOrder[i], want[i])
		}
	}
}

func TestWantsProviderInstall(t *testing.T) {
	spec := fleetv1alpha1.FleetClusterJoinSpec{
		Providers: fleetv1alpha1.ClusterProvidersSpec{
			Install: fleetv1alpha1.ProviderInstallSpec{
				Kubevirt: true,
				K8s:      true,
			},
		},
	}
	if !spec.WantsProviderInstall(fleetv1alpha1.ProviderKindKubevirt) {
		t.Fatal("expected kubevirt install")
	}
	if !spec.WantsProviderInstall(fleetv1alpha1.ProviderKindK8s) {
		t.Fatal("expected k8s install")
	}
	if spec.WantsProviderInstall(fleetv1alpha1.ProviderKindHelm) {
		t.Fatal("did not expect helm install")
	}
}

func TestProviderInstallSetIntent(t *testing.T) {
	var install fleetv1alpha1.ProviderInstallSpec
	install.SetIntent(fleetv1alpha1.ProviderKindK8s, true)
	if !install.K8s {
		t.Fatal("expected k8s intent true")
	}
	install.SetIntent(fleetv1alpha1.ProviderKindK8s, false)
	if install.K8s {
		t.Fatal("expected k8s intent false after delete")
	}
	install.SetIntent(fleetv1alpha1.ProviderKindHelm, true)
	if !install.Helm {
		t.Fatal("expected helm intent true")
	}
}

func TestProviderNameForKind(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName: "lab1",
		},
	}
	if got := providerNameForKind(fcj, "lab1", fleetv1alpha1.ProviderKindKubevirt); got != "kubevirt-lab1" {
		t.Fatalf("kubevirt name %q", got)
	}
	if got := providerNameForKind(fcj, "lab1", fleetv1alpha1.ProviderKindK8s); got != "k8s-lab1" {
		t.Fatalf("k8s name %q", got)
	}
}

func TestEnsureProviderStatusDefaultsAllKinds(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{}
	ensureProviderStatusDefaults(fcj, "lab1")
	for _, kind := range allProviderKinds() {
		st, ok := fcj.Status.Providers[kind]
		if !ok {
			t.Fatalf("missing status for %s", kind)
		}
		if st.Phase != fleetv1alpha1.ProviderPhaseNotInstalled {
			t.Fatalf("%s phase %s", kind, st.Phase)
		}
	}
}
