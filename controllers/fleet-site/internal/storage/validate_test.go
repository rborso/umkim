package storage

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateNFSSpecAllowsMissingDelegateForRemoteServer(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName:   "c1",
			BootstrapHost: "198.51.100.10",
		},
	}
	st := &fleetv1alpha1.ClusterStorageSpec{
		Backend: fleetv1alpha1.StorageBackendNFSExternal,
		NFS:     &fleetv1alpha1.ClusterNFSSpec{Server: "10.10.0.3"},
	}
	if err := ValidateNFSSpec(st, fcj); err != nil {
		t.Fatalf("external NFS without delegate should be allowed: %v", err)
	}
}

func TestValidateNFSSpecLocalServerOK(t *testing.T) {
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName:   "c1",
			BootstrapHost: "10.10.0.3",
		},
	}
	st := &fleetv1alpha1.ClusterStorageSpec{
		Backend: fleetv1alpha1.StorageBackendNFSExternal,
		NFS:     &fleetv1alpha1.ClusterNFSSpec{Server: "10.10.0.3"},
	}
	if err := ValidateNFSSpec(st, fcj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
