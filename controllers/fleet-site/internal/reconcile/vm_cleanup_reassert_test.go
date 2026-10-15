package reconcile

import (
	"context"
	"strings"
	"testing"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// A destroy completion triggers an inventory sync, and removing a declaration
// while one is in flight can be undone by it. The final re-assert pass runs after
// every destroy has settled, so a declaration a racing sync put back is removed
// again — and it stays idempotent for guests that are already gone.
func TestVmCleanupReassertsDeclarations(t *testing.T) {
	ns := "fleet-console"
	fcj := vmCleanupCluster(ns, "lab", map[string]string{annotationCleanupVmDecls: "true"})
	inv := vmCleanupInventory(ns, `{"guests":{"cp0":{"phase":"Declared"},"cp1":{"phase":"Declared"}}}`)
	// cp0 was removed earlier and a sync put it back; the group file still lists both.
	inv.Data["file.guests.yml"] = "guests:\n  hosts:\n    cp0:\n      ansible_host: 198.51.100.11\n"
	inv.Data["file.lab.yml"] = "lab:\n  hosts:\n    cp0: {}\n    cp1: {}\n"
	r, c := newVmCleanupTestReconciler(t, fcj, vmCleanupSite(ns, fleetv1alpha1.ProvisionProviderYaidap), inv)

	st := &fleetv1alpha1.ClusterVmCleanupStatus{
		Phase: fleetv1alpha1.VmCleanupPhaseDestroying,
		Hosts: []string{"cp0", "cp1"},
	}
	r.reassertVmCleanupDeclarations(context.Background(), fcj, st)

	if len(st.Failures) != 0 {
		t.Fatalf("unexpected failures: %v", st.Failures)
	}
	var cm corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Name: connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap), Namespace: ns,
	}, &cm); err != nil {
		t.Fatal(err)
	}
	if guests := cm.Data["file.guests.yml"]; strings.Contains(guests, "cp0:") || strings.Contains(guests, "cp1:") {
		t.Fatalf("declarations survived the re-assert:\n%s", guests)
	}
	if group, ok := cm.Data["file.lab.yml"]; ok && strings.Contains(group, "cp0:") {
		t.Fatalf("cluster group file still lists cp0:\n%s", group)
	}
}

// A probe Job whose pod is gone keeps no condition; the check must not wait on it
// forever. A Job with its own deadline is bounded by that, an older one by the
// age bound.
func TestOverlayCheckJobExpired(t *testing.T) {
	ns := "fleet-console"
	r, c := newVmCleanupTestReconciler(t)
	old := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	fresh := metav1.NewTime(time.Now().Add(-5 * time.Second))

	mk := func(name string, start *metav1.Time, deadline *int64) *batchv1.Job {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       batchv1.JobSpec{ActiveDeadlineSeconds: deadline},
			Status:     batchv1.JobStatus{StartTime: start},
		}
	}
	noDeadline := func() *int64 { return nil }
	withDeadline := func(v int64) *int64 { return &v }

	for _, tc := range []struct {
		name    string
		job     *batchv1.Job
		expired bool
	}{
		{"old without deadline", mk("j1", &old, noDeadline()), true},
		{"fresh without deadline", mk("j2", &fresh, noDeadline()), false},
		{"old beyond its deadline", mk("j3", &old, withDeadline(120)), true},
		{"fresh within its deadline", mk("j4", &fresh, withDeadline(120)), false},
		{"no start time", mk("j5", nil, noDeadline()), false},
	} {
		if err := c.Create(context.Background(), tc.job); err != nil {
			t.Fatalf("%s: create: %v", tc.name, err)
		}
		if got := r.overlayCheckJobExpired(context.Background(), ns, tc.job.Name); got != tc.expired {
			t.Errorf("%s: expired=%v want %v", tc.name, got, tc.expired)
		}
	}
	// A Job that is gone entirely must not keep the check waiting.
	if !r.overlayCheckJobExpired(context.Background(), ns, "missing") {
		t.Error("missing job should report expired")
	}
}
