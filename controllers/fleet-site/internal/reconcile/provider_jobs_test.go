package reconcile

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

func TestRecoverOrphanedProviderJobsInstall(t *testing.T) {
	providers := map[string]fleetv1alpha1.ClusterProviderStatus{
		fleetv1alpha1.ProviderKindK8s: {
			Phase:        fleetv1alpha1.ProviderPhaseInstalling,
			JobName:      "fleet-cluster-provider-k8s-install-lab1",
			ProviderName: "k8s-lab1",
			Message:      "Installing k8s provider",
		},
		fleetv1alpha1.ProviderKindHelm: {
			Phase:        fleetv1alpha1.ProviderPhaseReady,
			ProviderName: "helm-lab1",
		},
	}
	out, changed := recoverOrphanedProviderStatuses(providers, map[string]struct{}{
		fleetv1alpha1.ProviderKindK8s: {},
	})
	if !changed {
		t.Fatal("expected recovery")
	}
	k8s := out[fleetv1alpha1.ProviderKindK8s]
	if k8s.JobName != "" {
		t.Fatalf("jobName %q", k8s.JobName)
	}
	if k8s.Phase != fleetv1alpha1.ProviderPhaseNotInstalled {
		t.Fatalf("phase %s", k8s.Phase)
	}
	if k8s.Message != "Provider install job missing; retrying" {
		t.Fatalf("message %q", k8s.Message)
	}
	if out[fleetv1alpha1.ProviderKindHelm].Phase != fleetv1alpha1.ProviderPhaseReady {
		t.Fatal("helm status should be unchanged")
	}
}

func TestRecoverOrphanedProviderJobsDelete(t *testing.T) {
	providers := map[string]fleetv1alpha1.ClusterProviderStatus{
		fleetv1alpha1.ProviderKindK8s: {
			Phase:   fleetv1alpha1.ProviderPhaseDeleting,
			JobName: "fleet-cluster-provider-k8s-delete-lab1",
		},
	}
	out, changed := recoverOrphanedProviderStatuses(providers, map[string]struct{}{
		fleetv1alpha1.ProviderKindK8s: {},
	})
	if !changed {
		t.Fatal("expected recovery")
	}
	k8s := out[fleetv1alpha1.ProviderKindK8s]
	if k8s.Phase != fleetv1alpha1.ProviderPhaseNotInstalled {
		t.Fatalf("phase %s", k8s.Phase)
	}
	if k8s.Message != "Provider removed" {
		t.Fatalf("message %q", k8s.Message)
	}
}

func TestRecoverOrphanedProviderJobsNoOp(t *testing.T) {
	providers := map[string]fleetv1alpha1.ClusterProviderStatus{
		fleetv1alpha1.ProviderKindK8s: {
			Phase:   fleetv1alpha1.ProviderPhaseReady,
			JobName: "",
		},
	}
	_, changed := recoverOrphanedProviderStatuses(providers, map[string]struct{}{})
	if changed {
		t.Fatal("expected no recovery")
	}
}
