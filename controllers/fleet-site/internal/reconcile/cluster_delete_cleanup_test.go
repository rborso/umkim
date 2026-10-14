package reconcile

import (
	"context"
	"strings"
	"testing"

	"github.com/rborso/umkim/controllers/fleet-site/internal/teardown"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Storage cleanup is a node-side step inside the delete job, so the flag has to
// reach the job's extra vars or the export directory survives the delete.
func TestReconcileClusterDeletePassesStorageCleanup(t *testing.T) {
	ns := "fleet-console"
	fcj := deletingClusterJoin(ns, "lab")
	delete(fcj.Annotations, annotationDeleteJobSucceeded)
	fcj.Annotations[annotationCleanupStorage] = "true"
	r, c := newDeleteTestReconciler(t, fcj, clusterSSHSecret(ns, "lab"))

	if _, err := r.reconcileClusterDelete(context.Background(), fcj, "lab"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var job batchv1.Job
	if err := c.Get(context.Background(), types.NamespacedName{Name: "fleet-cluster-delete-lab", Namespace: ns}, &job); err != nil {
		t.Fatalf("delete job: %v", err)
	}
	// ansibleJob passes the playbook invocation as the container command.
	cmd := strings.Join(job.Spec.Template.Spec.Containers[0].Command, " ")
	if !strings.Contains(cmd, "fleet_cluster_cleanup_storage=true") {
		t.Fatalf("storage cleanup flag missing from job command:\n%s", cmd)
	}
}

// Without the annotation the delete job must not touch the storage host.
func TestReconcileClusterDeleteOmitsStorageCleanupByDefault(t *testing.T) {
	ns := "fleet-console"
	fcj := deletingClusterJoin(ns, "lab")
	delete(fcj.Annotations, annotationDeleteJobSucceeded)
	r, c := newDeleteTestReconciler(t, fcj, clusterSSHSecret(ns, "lab"))

	if _, err := r.reconcileClusterDelete(context.Background(), fcj, "lab"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var job batchv1.Job
	if err := c.Get(context.Background(), types.NamespacedName{Name: "fleet-cluster-delete-lab", Namespace: ns}, &job); err != nil {
		t.Fatalf("delete job: %v", err)
	}
	cmd := strings.Join(job.Spec.Template.Spec.Containers[0].Command, " ")
	if strings.Contains(cmd, "fleet_cluster_cleanup_storage") {
		t.Fatalf("storage cleanup must be opt-in:\n%s", cmd)
	}
}

// The per-cluster hub Secrets all go with the cluster: bootstrap, ansible-ssh,
// the ingress-auth stack secret, and the spoke kubeconfig.
func TestDeleteClusterJoinArtifactsRemovesClusterSecrets(t *testing.T) {
	ns := "fleet-console"
	objs := []client.Object{
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "fleet-cluster-lab-bootstrap", Namespace: ns}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "fleet-cluster-lab-ansible-ssh", Namespace: ns}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "fleet-ingress-auth-lab", Namespace: ns}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "fleet-kubeconfig-lab", Namespace: "fleet-system"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "fleet-ingress-auth-other", Namespace: ns}},
	}
	r, c := newDeleteTestReconciler(t, objs...)
	if err := teardown.DeleteClusterJoinArtifacts(context.Background(), c, r.Cfg, "lab"); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	for _, gone := range []types.NamespacedName{
		{Name: "fleet-cluster-lab-bootstrap", Namespace: ns},
		{Name: "fleet-cluster-lab-ansible-ssh", Namespace: ns},
		{Name: "fleet-ingress-auth-lab", Namespace: ns},
		{Name: "fleet-kubeconfig-lab", Namespace: "fleet-system"},
	} {
		var sec corev1.Secret
		if err := c.Get(context.Background(), gone, &sec); err == nil {
			t.Fatalf("%s/%s should have been removed", gone.Namespace, gone.Name)
		}
	}
	var other corev1.Secret
	if err := c.Get(context.Background(), types.NamespacedName{Name: "fleet-ingress-auth-other", Namespace: ns}, &other); err != nil {
		t.Fatalf("another cluster's ingress-auth secret must survive: %v", err)
	}
}

// Every cluster-scoped ConfigMap goes, not just the inventory one: the discover
// CM was missed by the hand-maintained list.
func TestDeleteClusterJoinArtifactsRemovesClusterCMs(t *testing.T) {
	ns := "fleet-console"
	objs := []client.Object{
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "fleet-cluster-lab-inventory", Namespace: ns}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "fleet-cluster-lab-discover", Namespace: ns}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "fleet-cluster-other-inventory", Namespace: ns}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "fleet-app-lab-values", Namespace: ns}},
	}
	r, c := newDeleteTestReconciler(t, objs...)
	if err := teardown.DeleteClusterJoinArtifacts(context.Background(), c, r.Cfg, "lab"); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	for _, gone := range []string{"fleet-cluster-lab-inventory", "fleet-cluster-lab-discover"} {
		var cm corev1.ConfigMap
		if err := c.Get(context.Background(), types.NamespacedName{Name: gone, Namespace: ns}, &cm); err == nil {
			t.Fatalf("%s should have been removed", gone)
		}
	}
	for _, kept := range []string{"fleet-cluster-other-inventory", "fleet-app-lab-values"} {
		var cm corev1.ConfigMap
		if err := c.Get(context.Background(), types.NamespacedName{Name: kept, Namespace: ns}, &cm); err != nil {
			t.Fatalf("%s must survive: %v", kept, err)
		}
	}
}

func clusterSSHSecret(ns, clusterName string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: clusterSSHSecretName(clusterName), Namespace: ns},
		Data:       map[string][]byte{"id_rsa": []byte("test-key")},
	}
}
