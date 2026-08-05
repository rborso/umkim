package storage

import (
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseNFSValidateJobLogs(t *testing.T) {
	logs := `TASK [Emit] ***
ok: [virt01] => {
    "msg": "UMKIM_NFS_VALIDATE_JSON:{\"ok\":false,\"checks\":[{\"name\":\"nfs_reachable\",\"ok\":false,\"message\":\"timeout\"}],\"nfsPath\":\"/srv/fleet-nfs/virt01\",\"node\":\"virt01\"}"
}`
	res, ok := ParseNFSValidateJobLogs(logs)
	if !ok || res == nil {
		t.Fatal("expected parsed result")
	}
	if res.OK {
		t.Fatal("expected ok=false")
	}
	if len(res.Checks) != 1 || res.Checks[0].Name != "nfs_reachable" {
		t.Fatalf("unexpected checks: %+v", res.Checks)
	}
	if res.NFSPath != "/srv/fleet-nfs/virt01" {
		t.Fatalf("nfsPath: %s", res.NFSPath)
	}
}

func TestValidateNFSSpecProbeAllowsMissingDelegate(t *testing.T) {
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
	if err := ValidateNFSSpecProbe(st, fcj); err != nil {
		t.Fatalf("probe should not require delegate: %v", err)
	}
}

func TestNFSSpecExtraArgs(t *testing.T) {
	st := &fleetv1alpha1.ClusterStorageSpec{
		Backend: fleetv1alpha1.StorageBackendNFSExternal,
		NFS: &fleetv1alpha1.ClusterNFSSpec{
			Server:       "10.10.0.3",
			ExportBase:   "/data/nfs",
			DelegateHost: "10.10.0.3",
			MountPath:    "/data/nfs/virt01",
		},
	}
	args := NFSSpecExtraArgs(st, "virt01")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"cluster_storage_nfs_server=10.10.0.3",
		"cluster_storage_nfs_export_base=/data/nfs",
		"cluster_storage_nfs_cluster_path=virt01",
		"cluster_storage_nfs_delegate_host=10.10.0.3",
		"cluster_storage_nfs_mount_path=/data/nfs/virt01",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}

	noDelegate := &fleetv1alpha1.ClusterStorageSpec{
		Backend: fleetv1alpha1.StorageBackendNFSExternal,
		NFS:     &fleetv1alpha1.ClusterNFSSpec{Server: "10.10.0.3"},
	}
	noDelegateArgs := strings.Join(NFSSpecExtraArgs(noDelegate, "virt01"), " ")
	if !strings.Contains(noDelegateArgs, "fleet_cluster_nfs_delegate_host=") {
		t.Fatalf("expected delegate cleared in %q", noDelegateArgs)
	}
}
