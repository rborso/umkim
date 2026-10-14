package reconcile

import (
	"context"
	"strings"
	"testing"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func clusterApp(name, cluster string, deleting bool) *fleetv1alpha1.FleetApp {
	app := &fleetv1alpha1.FleetApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "fleet-console",
			Finalizers: []string{fleetv1alpha1.FleetAppFinalizer},
		},
		Spec: fleetv1alpha1.FleetAppSpec{ClusterName: cluster, Chart: "immich", Version: "0.1.3"},
	}
	if deleting {
		now := metav1.Now()
		app.DeletionTimestamp = &now
	}
	return app
}

func appsCleanupCluster(ns, name string, extra map[string]string) *fleetv1alpha1.FleetClusterJoin {
	fcj := deletingClusterJoin(ns, name)
	fcj.Annotations[annotationCleanupApps] = "true"
	for k, v := range extra {
		fcj.Annotations[k] = v
	}
	return fcj
}

func TestAppsCleanupSkippedWithoutFlag(t *testing.T) {
	ns := "fleet-console"
	fcj := deletingClusterJoin(ns, "lab")
	r, c := newDeleteTestReconciler(t, fcj, clusterApp("immich", "lab", false))

	done, _, err := r.reconcileClusterDeleteApps(context.Background(), fcj, "lab")
	if err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	var got fleetv1alpha1.FleetClusterJoin
	if err := c.Get(context.Background(), types.NamespacedName{Name: "lab", Namespace: ns}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.AppsCleanup != nil {
		t.Fatalf("expected no apps cleanup status, got %+v", got.Status.AppsCleanup)
	}
	var app fleetv1alpha1.FleetApp
	if err := c.Get(context.Background(), types.NamespacedName{Name: "immich", Namespace: ns}, &app); err != nil {
		t.Fatal(err)
	}
	if !app.DeletionTimestamp.IsZero() {
		t.Fatal("app must not be touched without the flag")
	}
}

func TestAppsCleanupDoneWithoutApps(t *testing.T) {
	ns := "fleet-console"
	fcj := appsCleanupCluster(ns, "lab", nil)
	r, _ := newDeleteTestReconciler(t, fcj)

	done, _, err := r.reconcileClusterDeleteApps(context.Background(), fcj, "lab")
	if err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	st := fcj.Status.AppsCleanup
	if st == nil || st.Phase != fleetv1alpha1.AppsCleanupPhaseDone {
		t.Fatalf("status=%+v", st)
	}
	if st.Message != "no applications on this cluster" {
		t.Fatalf("message=%q", st.Message)
	}
}

// Only the cluster's own apps are uninstalled; another cluster's app is untouched.
func TestAppsCleanupDeletesOnlyThisClustersApps(t *testing.T) {
	ns := "fleet-console"
	fcj := appsCleanupCluster(ns, "lab", nil)
	r, c := newDeleteTestReconciler(t, fcj,
		clusterApp("immich", "lab", false),
		clusterApp("gitea", "other", false),
	)

	done, wait, err := r.reconcileClusterDeleteApps(context.Background(), fcj, "lab")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if done || wait <= 0 {
		t.Fatalf("expected a requeue while uninstalling, done=%v wait=%v", done, wait)
	}
	st := fcj.Status.AppsCleanup
	if strings.Join(st.Apps, ",") != "immich" {
		t.Fatalf("apps=%v want [immich]", st.Apps)
	}
	if st.Phase != fleetv1alpha1.AppsCleanupPhaseUninstalling {
		t.Fatalf("phase=%q", st.Phase)
	}
	var immich fleetv1alpha1.FleetApp
	if err := c.Get(context.Background(), types.NamespacedName{Name: "immich", Namespace: ns}, &immich); err != nil {
		t.Fatal(err)
	}
	if immich.DeletionTimestamp.IsZero() {
		t.Fatal("expected the cluster's app to be deleted (finalizer pending)")
	}
	var gitea fleetv1alpha1.FleetApp
	if err := c.Get(context.Background(), types.NamespacedName{Name: "gitea", Namespace: ns}, &gitea); err != nil {
		t.Fatal(err)
	}
	if !gitea.DeletionTimestamp.IsZero() {
		t.Fatal("another cluster's app must not be touched")
	}
}

func TestAppsCleanupCompletesWhenAppsAreGone(t *testing.T) {
	ns := "fleet-console"
	fcj := appsCleanupCluster(ns, "lab", nil)
	fcj.Status.AppsCleanup = &fleetv1alpha1.ClusterAppsCleanupStatus{
		Phase: fleetv1alpha1.AppsCleanupPhaseUninstalling,
		Apps:  []string{"immich", "gitea"},
	}
	r, _ := newDeleteTestReconciler(t, fcj)

	done, _, err := r.reconcileClusterDeleteApps(context.Background(), fcj, "lab")
	if err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	st := fcj.Status.AppsCleanup
	if st.Phase != fleetv1alpha1.AppsCleanupPhaseDone {
		t.Fatalf("phase=%q", st.Phase)
	}
	if strings.Join(st.Completed, ",") != "immich,gitea" {
		t.Fatalf("completed=%v", st.Completed)
	}
}

// The apps stage runs before the Ansible delete job: the Helm uninstalls need the
// spoke reachable, so a Kubernetes reset must not start first.
func TestReconcileClusterDeleteUninstallsAppsBeforeKubernetesReset(t *testing.T) {
	ns := "fleet-console"
	fcj := appsCleanupCluster(ns, "lab", nil)
	// No delete-job-succeeded marker: without the apps stage the delete job would
	// start on this pass.
	delete(fcj.Annotations, annotationDeleteJobSucceeded)
	r, c := newDeleteTestReconciler(t, fcj, clusterApp("immich", "lab", false))

	res, err := r.reconcileClusterDelete(context.Background(), fcj, "lab")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter <= 0 {
		t.Fatalf("expected a requeue while apps uninstall, got %+v", res)
	}
	var got fleetv1alpha1.FleetClusterJoin
	if err := c.Get(context.Background(), types.NamespacedName{Name: "lab", Namespace: ns}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.DeleteJobName != "" {
		t.Fatalf("delete job started before apps were uninstalled: %q", got.Status.DeleteJobName)
	}
	if got.Status.AppsCleanup == nil || got.Status.AppsCleanup.Phase != fleetv1alpha1.AppsCleanupPhaseUninstalling {
		t.Fatalf("apps cleanup status=%+v", got.Status.AppsCleanup)
	}
	var app fleetv1alpha1.FleetApp
	if err := c.Get(context.Background(), types.NamespacedName{Name: "immich", Namespace: ns}, &app); err != nil {
		t.Fatal(err)
	}
	if app.DeletionTimestamp.IsZero() {
		t.Fatal("expected the app to be uninstalling")
	}
}

// An app whose finalizer never finishes is reported, then the delete moves on.
func TestAppsCleanupGivesUpAfterTimeout(t *testing.T) {
	ns := "fleet-console"
	fcj := appsCleanupCluster(ns, "lab", nil)
	stuck := clusterApp("immich", "lab", false)
	started := metav1.NewTime(time.Now().Add(-2 * appsCleanupTimeout))
	fcj.Status.AppsCleanup = &fleetv1alpha1.ClusterAppsCleanupStatus{
		Phase:     fleetv1alpha1.AppsCleanupPhaseUninstalling,
		Apps:      []string{"immich"},
		StartedAt: &started,
	}
	r, c := newDeleteTestReconciler(t, fcj, stuck)

	done, _, err := r.reconcileClusterDeleteApps(context.Background(), fcj, "lab")
	if err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	st := fcj.Status.AppsCleanup
	if st.Phase != fleetv1alpha1.AppsCleanupPhaseFailed {
		t.Fatalf("phase=%q", st.Phase)
	}
	if len(st.Failures) != 1 || !strings.HasPrefix(st.Failures[0], "immich: ") {
		t.Fatalf("failures=%v", st.Failures)
	}
	// The app object is left alone so its uninstall can be retried.
	var app fleetv1alpha1.FleetApp
	if err := c.Get(context.Background(), types.NamespacedName{Name: "immich", Namespace: ns}, &app); err != nil {
		t.Fatalf("app should still exist: %v", err)
	}
}
