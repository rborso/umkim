package reconcile

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Deleting an app must not leave hub-side objects behind: the rendered values,
// the image map, and the directory bind credentials the controller keeps for the
// app's helm job. None of them are owned by Helm or the chart.
func TestAppDeleteRemovesHubObjects(t *testing.T) {
	ns := "fleet-console"
	now := metav1.Now()
	app := &fleetv1alpha1.FleetApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "gitea",
			Namespace:         ns,
			Finalizers:        []string{fleetv1alpha1.FleetAppFinalizer},
			DeletionTimestamp: &now,
		},
		Spec:   fleetv1alpha1.FleetAppSpec{ClusterName: "lab", Chart: "gitea", Version: "0.1.9", Namespace: "gitea"},
		Status: fleetv1alpha1.FleetAppStatus{Phase: fleetv1alpha1.AppPhaseDeleting},
	}
	objs := []client.Object{
		app,
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: appValuesCMName("gitea"), Namespace: ns}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: appImageMapCMName("gitea"), Namespace: ns}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: appDirectoryBindSecretName("gitea"), Namespace: ns}},
	}
	finishedUninstall := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: appJobName("fa-u-", "gitea"), Namespace: ns},
		Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{
			Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
		}}},
	}
	scheme := deleteTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&fleetv1alpha1.FleetApp{}, &batchv1.Job{}).
		WithObjects(append(objs, finishedUninstall)...).
		Build()
	r := &AppReconciler{Reconciler: &Reconciler{
		Client: c,
		Scheme: scheme,
		Cfg:    config.Config{Namespace: ns},
	}}

	if _, err := r.reconcileDelete(context.Background(), app); err != nil {
		t.Fatalf("reconcileDelete: %v", err)
	}
	for _, gone := range []struct {
		kind string
		name string
	}{
		{"configmap", appValuesCMName("gitea")},
		{"configmap", appImageMapCMName("gitea")},
		{"secret", appDirectoryBindSecretName("gitea")},
	} {
		switch gone.kind {
		case "configmap":
			var cm corev1.ConfigMap
			if err := c.Get(context.Background(), types.NamespacedName{Name: gone.name, Namespace: ns}, &cm); err == nil {
				t.Fatalf("%s %s should have been removed", gone.kind, gone.name)
			}
		case "secret":
			var sec corev1.Secret
			if err := c.Get(context.Background(), types.NamespacedName{Name: gone.name, Namespace: ns}, &sec); err == nil {
				t.Fatalf("%s %s should have been removed", gone.kind, gone.name)
			}
		}
	}
}
