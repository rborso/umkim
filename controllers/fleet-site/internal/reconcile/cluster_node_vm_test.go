package reconcile

import (
	"context"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func nodeVMTestReconciler(t *testing.T, objs ...client.Object) (*ClusterJoinReconciler, client.Client) {
	t.Helper()
	scheme := deleteTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &ClusterJoinReconciler{Reconciler: &Reconciler{
		Client: c,
		Scheme: scheme,
		Cfg:    config.Config{Namespace: "fleet-console"},
	}}, c
}

func nodeVMTestSite() *fleetv1alpha1.FleetSite {
	return &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: "fleet-console"},
		Spec:       fleetv1alpha1.FleetSiteSpec{SiteName: "homelab", SiteId: "homelab"},
	}
}

func nodeVMTestFCJ() *fleetv1alpha1.FleetClusterJoin {
	return &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "lab-vm", Namespace: "fleet-console"},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName:   "lab-vm",
			SiteId:        "homelab",
			MinRootDiskGi: 40,
		},
	}
}

func nodeVMTestNode() fleetv1alpha1.FleetClusterJoinNodeSpec {
	return fleetv1alpha1.FleetClusterJoinNodeSpec{
		Hostname: "lab-vm-1",
		Address:  "172.16.10.101",
		Role:     fleetv1alpha1.NodeRoleControl,
		VM:       &fleetv1alpha1.FleetClusterJoinNodeVM{},
	}
}

func inventoryCM(t *testing.T, c client.Client) *corev1.ConfigMap {
	t.Helper()
	var cm corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Namespace: "fleet-console",
		Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
	}, &cm); err != nil {
		t.Fatal(err)
	}
	return &cm
}

func TestEnsureNodeGuestDeclaresGuest(t *testing.T) {
	r, c := nodeVMTestReconciler(t)
	node := nodeVMTestNode()
	node.VM = &fleetv1alpha1.FleetClusterJoinNodeVM{Cores: 4, Memory: 6144, DiskGi: 50}

	declared, err := r.ensureNodeGuest(context.Background(), nodeVMTestSite(), nodeVMTestFCJ(), node)
	if err != nil {
		t.Fatal(err)
	}
	if !declared {
		t.Fatal("want the guest to be declared")
	}
	data := inventoryCM(t, c).Data
	guests := data[connectors.FileKey("guests.yml")]
	if !strings.Contains(guests, "lab-vm-1") || !strings.Contains(guests, "172.16.10.101") {
		t.Fatalf("guests.yml = %q", guests)
	}
	hostVars := data[connectors.FileKey("host_vars/lab-vm-1.yml")]
	for _, want := range []string{"cores: 4", "memory: 6144", "size: 50"} {
		if !strings.Contains(hostVars, want) {
			t.Fatalf("host_vars missing %q:\n%s", want, hostVars)
		}
	}
	// The host is also added to the cluster group, which is what ties the guest
	// to the cluster name.
	group := data[connectors.FileKey("lab-vm.yml")]
	if !strings.Contains(group, "lab-vm-1") {
		t.Fatalf("cluster group = %q", group)
	}
}

func TestEnsureNodeGuestIsIdempotent(t *testing.T) {
	r, c := nodeVMTestReconciler(t)
	site, fcj, node := nodeVMTestSite(), nodeVMTestFCJ(), nodeVMTestNode()

	if _, err := r.ensureNodeGuest(context.Background(), site, fcj, node); err != nil {
		t.Fatal(err)
	}
	first := inventoryCM(t, c).ResourceVersion

	declared, err := r.ensureNodeGuest(context.Background(), site, fcj, node)
	if err != nil {
		t.Fatal(err)
	}
	if declared {
		t.Fatal("second declare must be a no-op")
	}
	if got := inventoryCM(t, c).ResourceVersion; got != first {
		t.Fatalf("inventory rewritten on the second call (rv %s -> %s)", first, got)
	}
}

func TestEnsureNodeGuestRejectsAddressMismatch(t *testing.T) {
	r, c := nodeVMTestReconciler(t)
	site, fcj, node := nodeVMTestSite(), nodeVMTestFCJ(), nodeVMTestNode()
	if _, err := r.ensureNodeGuest(context.Background(), site, fcj, node); err != nil {
		t.Fatal(err)
	}
	_ = c

	moved := nodeVMTestNode()
	moved.Address = "172.16.10.199"
	_, err := r.ensureNodeGuest(context.Background(), site, fcj, moved)
	if err == nil || !strings.Contains(err.Error(), "already declared") {
		t.Fatalf("want address-mismatch error, got %v", err)
	}
}

func TestEnsureNodeGuestSkipsNodeWithoutVMSpec(t *testing.T) {
	r, c := nodeVMTestReconciler(t)
	node := nodeVMTestNode()
	node.VM = nil

	declared, err := r.ensureNodeGuest(context.Background(), nodeVMTestSite(), nodeVMTestFCJ(), node)
	if err != nil {
		t.Fatal(err)
	}
	if declared {
		t.Fatal("a node without a vm spec must not be declared")
	}
	var cm corev1.ConfigMap
	err = c.Get(context.Background(), types.NamespacedName{
		Namespace: "fleet-console",
		Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
	}, &cm)
	if err == nil {
		t.Fatal("inventory must not be created for a node without a vm spec")
	}
}

func TestEnsureNodeGuestRaisesBootDiskToMinRootDisk(t *testing.T) {
	r, c := nodeVMTestReconciler(t)
	node := nodeVMTestNode()
	node.VM = &fleetv1alpha1.FleetClusterJoinNodeVM{DiskGi: 20}

	if _, err := r.ensureNodeGuest(context.Background(), nodeVMTestSite(), nodeVMTestFCJ(), node); err != nil {
		t.Fatal(err)
	}
	hostVars := inventoryCM(t, c).Data[connectors.FileKey("host_vars/lab-vm-1.yml")]
	// minRootDiskGi (40) + the EFI/boot overhead: a disk sized exactly at the
	// threshold reports a smaller root filesystem and fails the bootstrap.
	if !strings.Contains(hostVars, "size: 42") {
		t.Fatalf("boot disk must exceed minRootDiskGi by the boot overhead:\n%s", hostVars)
	}
}

func TestEnsureNodeGuestKeepsLargerBootDisk(t *testing.T) {
	r, c := nodeVMTestReconciler(t)
	node := nodeVMTestNode()
	node.VM = &fleetv1alpha1.FleetClusterJoinNodeVM{DiskGi: 100}

	if _, err := r.ensureNodeGuest(context.Background(), nodeVMTestSite(), nodeVMTestFCJ(), node); err != nil {
		t.Fatal(err)
	}
	hostVars := inventoryCM(t, c).Data[connectors.FileKey("host_vars/lab-vm-1.yml")]
	if !strings.Contains(hostVars, "size: 100") {
		t.Fatalf("an explicit larger boot disk must be kept:\n%s", hostVars)
	}
}

func TestNodeVMDataDiskGi(t *testing.T) {
	fcj := nodeVMTestFCJ()
	node := nodeVMTestNode()

	if got := nodeVMDataDiskGi(fcj, node); got != 0 {
		t.Fatalf("no longhorn and no explicit disk: got %d want 0", got)
	}

	explicit := node
	explicit.VM = &fleetv1alpha1.FleetClusterJoinNodeVM{DataDiskGi: 77}
	if got := nodeVMDataDiskGi(fcj, explicit); got != 77 {
		t.Fatalf("explicit data disk: got %d want 77", got)
	}

	fcj.Spec.Storage = &fleetv1alpha1.ClusterStorageSpec{
		Longhorn: &fleetv1alpha1.ClusterLonghornSpec{Enabled: true},
	}
	if got := nodeVMDataDiskGi(fcj, node); got != defaultNodeVMDataDiskGi {
		t.Fatalf("longhorn default: got %d want %d", got, defaultNodeVMDataDiskGi)
	}

	fcj.Spec.Storage.MinDataDiskGi = 25
	if got := nodeVMDataDiskGi(fcj, node); got != 25 {
		t.Fatalf("longhorn minDataDiskGi: got %d want 25", got)
	}
}

func TestEnsureNodeGuestAddsLonghornDataDisk(t *testing.T) {
	r, c := nodeVMTestReconciler(t)
	fcj := nodeVMTestFCJ()
	fcj.Spec.Storage = &fleetv1alpha1.ClusterStorageSpec{
		Longhorn: &fleetv1alpha1.ClusterLonghornSpec{Enabled: true},
	}
	if _, err := r.ensureNodeGuest(context.Background(), nodeVMTestSite(), fcj, nodeVMTestNode()); err != nil {
		t.Fatal(err)
	}
	hostVars := inventoryCM(t, c).Data[connectors.FileKey("host_vars/lab-vm-1.yml")]
	if !strings.Contains(hostVars, "disk1") || !strings.Contains(hostVars, "vdb") {
		t.Fatalf("data disk missing from host_vars:\n%s", hostVars)
	}
}

func TestDeclaredGuestAddress(t *testing.T) {
	if _, ok, err := declaredGuestAddress("", "x"); ok || err != nil {
		t.Fatalf("empty guests.yml: ok=%v err=%v", ok, err)
	}
	content := "guests:\n  hosts:\n    lab-vm-1:\n      ansible_host: 172.16.10.101\n"
	addr, ok, err := declaredGuestAddress(content, "lab-vm-1")
	if err != nil || !ok || addr != "172.16.10.101" {
		t.Fatalf("addr=%q ok=%v err=%v", addr, ok, err)
	}
	if _, ok, _ := declaredGuestAddress(content, "other"); ok {
		t.Fatal("unknown host must not be reported as declared")
	}
}
