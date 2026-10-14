package reconcile

import (
	"context"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newVmCleanupTestReconciler also registers FleetSite as a status subresource:
// starting a destroy action commits progress to the site's provision status.
func newVmCleanupTestReconciler(t *testing.T, objs ...client.Object) (*ClusterJoinReconciler, client.Client) {
	t.Helper()
	scheme := deleteTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&fleetv1alpha1.FleetClusterJoin{}, &fleetv1alpha1.FleetSite{}, &batchv1.Job{}).
		WithObjects(objs...).
		Build()
	r := &ClusterJoinReconciler{Reconciler: &Reconciler{
		Client: c,
		Scheme: scheme,
		Cfg:    config.Config{Namespace: "fleet-console"},
	}}
	return r, c
}

const vmCleanupGuestsYAML = `guests:
  hosts:
    cp0:
      ansible_host: 198.51.100.11
      vm_location: virt01
    cp1:
      ansible_host: 198.51.100.12
      vm_location: virt01
`

func vmCleanupSite(ns, provider string) *fleetv1alpha1.FleetSite {
	return &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName: "homelab",
			SiteId:   "homelab",
			Provision: &fleetv1alpha1.SiteProvisionSpec{
				Provider:      provider,
				Jumphost:      "linux@jump.example.com",
				InventoryPath: "/srv/yaidap/inventory",
			},
		},
	}
}

func vmCleanupInventory(ns, statusJSON string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: ns,
		},
		Data: map[string]string{
			"file.guests.yml":   vmCleanupGuestsYAML,
			"_meta.status.json": statusJSON,
		},
	}
}

// vmCleanupCluster builds a Deleting cluster whose nodes are the two VM guests.
func vmCleanupCluster(ns, name string, extraAnnotations map[string]string) *fleetv1alpha1.FleetClusterJoin {
	fcj := deletingClusterJoin(ns, name)
	fcj.Annotations[annotationCleanupVms] = "true"
	for k, v := range extraAnnotations {
		fcj.Annotations[k] = v
	}
	fcj.Spec.Nodes = []fleetv1alpha1.FleetClusterJoinNodeSpec{
		{Hostname: "cp0", Address: "198.51.100.11", Role: fleetv1alpha1.NodeRoleControl},
		{Hostname: "cp1", Address: "198.51.100.12", Role: fleetv1alpha1.NodeRoleControl},
		{Hostname: "bm0", Address: "198.51.100.13", Role: fleetv1alpha1.NodeRoleWorker},
	}
	fcj.Status.Nodes = nil
	return fcj
}

func TestVmCleanupSkippedWithoutFlag(t *testing.T) {
	ns := "fleet-console"
	fcj := deletingClusterJoin(ns, "lab")
	r, c := newVmCleanupTestReconciler(t, fcj, vmCleanupSite(ns, fleetv1alpha1.ProvisionProviderYaidap))

	done, _, err := r.reconcileClusterDeleteVms(context.Background(), fcj)
	if err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	var got fleetv1alpha1.FleetClusterJoin
	if err := c.Get(context.Background(), types.NamespacedName{Name: "lab", Namespace: ns}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.VmCleanup != nil {
		t.Fatalf("expected no VM cleanup status, got %+v", got.Status.VmCleanup)
	}
}

func TestVmCleanupSkippedWithoutYaidapSite(t *testing.T) {
	ns := "fleet-console"
	fcj := vmCleanupCluster(ns, "lab", nil)
	r, _ := newVmCleanupTestReconciler(t, fcj, vmCleanupSite(ns, fleetv1alpha1.ProvisionProviderNoop))

	done, _, err := r.reconcileClusterDeleteVms(context.Background(), fcj)
	if err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	if fcj.Status.VmCleanup == nil || fcj.Status.VmCleanup.Phase != fleetv1alpha1.VmCleanupPhaseSkipped {
		t.Fatalf("expected Skipped, got %+v", fcj.Status.VmCleanup)
	}
}

// Only VM guests are destroyable: the bare-metal node is not in the work list.
func TestVmCleanupListsOnlyVmGuests(t *testing.T) {
	ns := "fleet-console"
	fcj := vmCleanupCluster(ns, "lab", nil)
	inv := vmCleanupInventory(ns, `{"guests":{"cp0":{"phase":"Ready"},"cp1":{"phase":"Ready"}}}`)
	r, _ := newVmCleanupTestReconciler(t, fcj, vmCleanupSite(ns, fleetv1alpha1.ProvisionProviderYaidap), inv)

	done, wait, err := r.reconcileClusterDeleteVms(context.Background(), fcj)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if done || wait <= 0 {
		t.Fatalf("expected a requeue while destroying, done=%v wait=%v", done, wait)
	}
	st := fcj.Status.VmCleanup
	if st == nil {
		t.Fatal("expected VM cleanup status")
	}
	if strings.Join(st.Hosts, ",") != "cp0,cp1" {
		t.Fatalf("hosts=%v want [cp0 cp1]", st.Hosts)
	}
	if st.Phase != fleetv1alpha1.VmCleanupPhaseDestroying {
		t.Fatalf("phase=%q", st.Phase)
	}
	if st.JobName == "" || st.ActionID == "" {
		t.Fatalf("expected a destroy action: job=%q id=%q", st.JobName, st.ActionID)
	}
}

// A guest whose VM is already gone (Declared) is finished without a destroy job;
// with declaration cleanup the inventory entry goes with it.
// With the declaration flag, a Declared guest whose destroy succeeds (the VM is
// really gone) is dropped from the inventory.
func TestVmCleanupRemovesDeclarationsAfterDestroy(t *testing.T) {
	ns := "fleet-console"
	fcj := vmCleanupCluster(ns, "lab", map[string]string{annotationCleanupVmDecls: "true"})
	inv := vmCleanupInventory(ns, `{"guests":{"cp0":{"phase":"Declared"},"cp1":{"phase":"Declared"}}}`)
	finished0 := &batchv1Job{name: "fleet-site-prov-a1", namespace: ns}
	fcj.Status.VmCleanup = &fleetv1alpha1.ClusterVmCleanupStatus{
		Phase:   fleetv1alpha1.VmCleanupPhaseDestroying,
		Hosts:   []string{"cp0", "cp1"},
		JobName: "fleet-site-prov-a1",
	}
	r, c := newVmCleanupTestReconciler(t, fcj,
		vmCleanupSite(ns, fleetv1alpha1.ProvisionProviderYaidap), inv, finished0.object())

	// Pass 1: cp0's destroy finishes and the stage starts one for cp1.
	if _, _, err := r.reconcileClusterDeleteVms(context.Background(), fcj); err != nil {
		t.Fatalf("reconcile 1: %v", err)
	}
	if !vmCleanupCompleted(fcj.Status.VmCleanup, "cp0") {
		t.Fatalf("cp0 should be done: %+v", fcj.Status.VmCleanup)
	}
	// Complete the destroy the stage started for cp1 (the connector job exists).
	cp1Job := fcj.Status.VmCleanup.JobName
	if cp1Job == "" {
		t.Fatalf("no destroy started for cp1: %+v", fcj.Status.VmCleanup)
	}
	var job batchv1.Job
	if err := c.Get(context.Background(), types.NamespacedName{Name: cp1Job, Namespace: ns}, &job); err != nil {
		t.Fatalf("cp1 job: %v", err)
	}
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
	if err := c.Status().Update(context.Background(), &job); err != nil {
		t.Fatalf("finish cp1 job: %v", err)
	}

	done, _, err := r.reconcileClusterDeleteVms(context.Background(), fcj)
	if err != nil {
		t.Fatalf("reconcile 2: %v", err)
	}
	if !done {
		t.Fatalf("expected cleanup to finish, status=%+v", fcj.Status.VmCleanup)
	}
	if fcj.Status.VmCleanup.Phase != fleetv1alpha1.VmCleanupPhaseDone {
		t.Fatalf("phase=%+v", fcj.Status.VmCleanup)
	}
	var cm corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Name: connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap), Namespace: ns,
	}, &cm); err != nil {
		t.Fatal(err)
	}
	if guests := cm.Data["file.guests.yml"]; strings.Contains(guests, "cp0:") || strings.Contains(guests, "cp1:") {
		t.Fatalf("declarations survived:\n%s", guests)
	}
}

// Without the declaration flag the guests stay in the inventory.
func TestVmCleanupKeepsDeclarationsByDefault(t *testing.T) {
	ns := "fleet-console"
	fcj := vmCleanupCluster(ns, "lab", nil)
	inv := vmCleanupInventory(ns, `{"guests":{"cp0":{"phase":"Declared"},"cp1":{"phase":"Declared"}}}`)
	finished := &batchv1Job{name: "fleet-site-prov-a1", namespace: ns}
	fcj.Status.VmCleanup = &fleetv1alpha1.ClusterVmCleanupStatus{
		Phase:   fleetv1alpha1.VmCleanupPhaseDestroying,
		Hosts:   []string{"cp0", "cp1"},
		JobName: "fleet-site-prov-a1",
	}
	r, c := newVmCleanupTestReconciler(t, fcj,
		vmCleanupSite(ns, fleetv1alpha1.ProvisionProviderYaidap), inv, finished.object())

	if _, _, err := r.reconcileClusterDeleteVms(context.Background(), fcj); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var cm corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Name: connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap), Namespace: ns,
	}, &cm); err != nil {
		t.Fatal(err)
	}
	if guests := cm.Data["file.guests.yml"]; !strings.Contains(guests, "cp0:") {
		t.Fatalf("declaration should be kept:\n%s", guests)
	}
}

// A finished destroy job advances the guest and starts the next one.
func TestVmCleanupAdvancesAfterDestroyJob(t *testing.T) {
	ns := "fleet-console"
	fcj := vmCleanupCluster(ns, "lab", nil)
	inv := vmCleanupInventory(ns, `{"guests":{"cp0":{"phase":"Destroying"},"cp1":{"phase":"Ready"}}}`)
	fcj.Status.VmCleanup = &fleetv1alpha1.ClusterVmCleanupStatus{
		Phase:    fleetv1alpha1.VmCleanupPhaseDestroying,
		Hosts:    []string{"cp0", "cp1"},
		ActionID: "abc123",
		JobName:  "fleet-site-prov-abc123",
	}
	finished := &batchv1Job{name: "fleet-site-prov-abc123", namespace: ns}
	r, c := newVmCleanupTestReconciler(t, fcj, vmCleanupSite(ns, fleetv1alpha1.ProvisionProviderYaidap), inv, finished.object())

	done, wait, err := r.reconcileClusterDeleteVms(context.Background(), fcj)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if done || wait <= 0 {
		t.Fatalf("expected a requeue, done=%v wait=%v", done, wait)
	}
	st := fcj.Status.VmCleanup
	if strings.Join(st.Completed, ",") != "cp0" {
		t.Fatalf("completed=%v want [cp0]", st.Completed)
	}
	var got fleetv1alpha1.FleetClusterJoin
	if err := c.Get(context.Background(), types.NamespacedName{Name: "lab", Namespace: ns}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.VmCleanup == nil || len(got.Status.VmCleanup.Completed) != 1 {
		t.Fatalf("status not persisted: %+v", got.Status.VmCleanup)
	}
}

// A failing destroy job is reported, retried once, then skipped so the delete can
// finish instead of wedging on a broken hypervisor.
func TestVmCleanupGivesUpAfterMaxAttempts(t *testing.T) {
	ns := "fleet-console"
	fcj := vmCleanupCluster(ns, "lab", nil)
	inv := vmCleanupInventory(ns, `{"guests":{"cp0":{"phase":"Destroying"},"cp1":{"phase":"Ready"}}}`)
	fcj.Status.VmCleanup = &fleetv1alpha1.ClusterVmCleanupStatus{
		Phase:    fleetv1alpha1.VmCleanupPhaseDestroying,
		Hosts:    []string{"cp0", "cp1"},
		JobName:  "fleet-site-prov-abc123",
		Failures: []string{"cp0: boom"},
	}
	failed := &batchv1Job{name: "fleet-site-prov-abc123", namespace: ns, failed: true}
	r, _ := newVmCleanupTestReconciler(t, fcj, vmCleanupSite(ns, fleetv1alpha1.ProvisionProviderYaidap), inv, failed.object())

	done, wait, err := r.reconcileClusterDeleteVms(context.Background(), fcj)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if done || wait <= 0 {
		t.Fatalf("expected a requeue, done=%v wait=%v", done, wait)
	}
	st := fcj.Status.VmCleanup
	if !vmCleanupCompleted(st, "cp0") {
		t.Fatalf("cp0 should be given up on: %+v", st)
	}
	if len(st.Failures) == 0 {
		t.Fatalf("failure should be recorded: %+v", st)
	}
}

// A guest with no status record must not be read as "already destroyed": the
// inventory view defaults it to Declared, which on a live delete silently skipped
// a running VM. With no recorded phase the destroy is attempted.
func TestVmCleanupAttemptsDestroyWhenPhaseIsUnknown(t *testing.T) {
	ns := "fleet-console"
	fcj := vmCleanupCluster(ns, "lab", nil)
	// guests.yml lists the guests; the status doc has no entry for them.
	inv := vmCleanupInventory(ns, `{"guests":{}}`)
	r, _ := newVmCleanupTestReconciler(t, fcj, vmCleanupSite(ns, fleetv1alpha1.ProvisionProviderYaidap), inv)

	done, wait, err := r.reconcileClusterDeleteVms(context.Background(), fcj)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if done || wait <= 0 {
		t.Fatalf("expected a destroy attempt, done=%v wait=%v status=%+v", done, wait, fcj.Status.VmCleanup)
	}
	st := fcj.Status.VmCleanup
	if st.JobName == "" {
		t.Fatalf("no destroy job started for an unknown-phase guest: %+v", st)
	}
	if strings.Join(st.Completed, ",") != "" {
		t.Fatalf("nothing should be completed yet: %+v", st.Completed)
	}
}

// A Declared guest is still attempted: the recorded phase has read Declared for a
// VM that was still running, and a skipped destroy leaves it behind.
func TestVmCleanupAttemptsDestroyForDeclaredGuest(t *testing.T) {
	ns := "fleet-console"
	fcj := vmCleanupCluster(ns, "lab", nil)
	inv := vmCleanupInventory(ns, `{"guests":{"cp0":{"phase":"Declared"},"cp1":{"phase":"Declared"}}}`)
	r, _ := newVmCleanupTestReconciler(t, fcj, vmCleanupSite(ns, fleetv1alpha1.ProvisionProviderYaidap), inv)

	done, wait, err := r.reconcileClusterDeleteVms(context.Background(), fcj)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if done || wait <= 0 {
		t.Fatalf("expected a destroy attempt, done=%v wait=%v status=%+v", done, wait, fcj.Status.VmCleanup)
	}
	if fcj.Status.VmCleanup.JobName == "" {
		t.Fatalf("no destroy started for a Declared guest: %+v", fcj.Status.VmCleanup)
	}
}

// When the phase said Declared and the destroy fails, that confirms the guest is
// gone: no failure is recorded and the declaration is kept (the phase has been
// wrong before, so it stays as the pointer to a possibly-live VM).
func TestVmCleanupDeclaredGuestFailureKeepsDeclaration(t *testing.T) {
	ns := "fleet-console"
	fcj := vmCleanupCluster(ns, "lab", map[string]string{annotationCleanupVmDecls: "true"})
	inv := vmCleanupInventory(ns, `{"guests":{"cp0":{"phase":"Declared"},"cp1":{"phase":"Declared"}}}`)
	fcj.Status.VmCleanup = &fleetv1alpha1.ClusterVmCleanupStatus{
		Phase:   fleetv1alpha1.VmCleanupPhaseDestroying,
		Hosts:   []string{"cp0", "cp1"},
		JobName: "fleet-site-prov-abc123",
	}
	failed := &batchv1Job{name: "fleet-site-prov-abc123", namespace: ns, failed: true}
	r, c := newVmCleanupTestReconciler(t, fcj, vmCleanupSite(ns, fleetv1alpha1.ProvisionProviderYaidap), inv, failed.object())

	done, wait, err := r.reconcileClusterDeleteVms(context.Background(), fcj)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if done || wait <= 0 {
		t.Fatalf("expected a requeue, done=%v wait=%v", done, wait)
	}
	st := fcj.Status.VmCleanup
	if len(st.Failures) != 0 {
		t.Fatalf("a Declared guest with a failed destroy is not a failure: %+v", st.Failures)
	}
	if !vmCleanupCompleted(st, "cp0") {
		t.Fatalf("cp0 should be finished: %+v", st)
	}
	var cm corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Name: connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap), Namespace: ns,
	}, &cm); err != nil {
		t.Fatal(err)
	}
	if guests := cm.Data["file.guests.yml"]; !strings.Contains(guests, "cp0:") {
		t.Fatalf("the declaration must be kept:\n%s", guests)
	}
}

// batchv1Job builds a connector job in either terminal state.
type batchv1Job struct {
	name      string
	namespace string
	failed    bool
}

func (j *batchv1Job) object() client.Object {
	cond := batchv1.JobCondition{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}
	if j.failed {
		cond = batchv1.JobCondition{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "boom"}
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: j.name, Namespace: j.namespace},
		Status:     batchv1.JobStatus{Conditions: []batchv1.JobCondition{cond}},
	}
}
