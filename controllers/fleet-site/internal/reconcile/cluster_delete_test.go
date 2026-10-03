package reconcile

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func deleteTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	}, &unstructured.Unstructured{})
	return scheme
}

func deletingClusterJoin(ns, name string) *fleetv1alpha1.FleetClusterJoin {
	return &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  ns,
			Finalizers: []string{fleetv1alpha1.ClusterCleanupFinalizer},
			Annotations: map[string]string{
				annotationDeleteJobSucceeded: "true",
			},
		},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName:   name,
			SiteId:        "homelab",
			BootstrapHost: "198.51.100.10",
		},
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			Phase: fleetv1alpha1.JoinPhaseDeleting,
			Nodes: []fleetv1alpha1.FleetClusterJoinNodeStatus{{
				Hostname:           name,
				Address:            "198.51.100.10",
				WireguardAddress:   "10.10.0.14/32",
				WireguardPublicKey: "wg-pub-key",
			}},
		},
	}
}

func finishedDeleteJob(ns, clusterName string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fleet-cluster-delete-" + clusterName,
			Namespace: ns,
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{{
				Type:   batchv1.JobComplete,
				Status: corev1.ConditionTrue,
			}},
		},
	}
}

func newDeleteTestReconciler(t *testing.T, objs ...client.Object) (*ClusterJoinReconciler, client.Client) {
	t.Helper()
	scheme := deleteTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&fleetv1alpha1.FleetClusterJoin{}, &batchv1.Job{}).
		WithObjects(objs...).
		Build()
	r := &ClusterJoinReconciler{Reconciler: &Reconciler{
		Client: c,
		Scheme: scheme,
		Cfg:    config.Config{Namespace: "fleet-console"},
	}}
	return r, c
}

func TestReconcileClusterDeleteContinuesWireGuardAfterSuccess(t *testing.T) {
	ns := "fleet-console"
	const clusterName = "lab"
	fcj := deletingClusterJoin(ns, clusterName)
	job := finishedDeleteJob(ns, clusterName)
	r, c := newDeleteTestReconciler(t, fcj, job)

	if _, err := r.reconcileClusterDelete(context.Background(), fcj, clusterName); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got fleetv1alpha1.FleetClusterJoin
	if err := c.Get(context.Background(), types.NamespacedName{Name: clusterName, Namespace: ns}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[annotationDeleteJobSucceeded] != "true" {
		t.Fatalf("delete-job-succeeded=%q", got.Annotations[annotationDeleteJobSucceeded])
	}
	if got.Status.DeleteJobName != "" {
		t.Fatalf("delete job name restarted: %q", got.Status.DeleteJobName)
	}
	if got.Status.WgRemoveJobName == "" {
		t.Fatal("expected wireguard removal job")
	}
	var still batchv1.Job
	if err := c.Get(context.Background(), types.NamespacedName{Name: job.Name, Namespace: ns}, &still); err != nil {
		t.Fatalf("finished delete job missing: %v", err)
	}
	done, ok := jobIsFinished(&still)
	if !done || !ok {
		t.Fatal("delete job was recreated instead of left complete")
	}
}

func TestReconcileClusterDeleteRetriesWhileManagedClusterExists(t *testing.T) {
	ns := "fleet-console"
	const clusterName = "lab"
	fcj := deletingClusterJoin(ns, clusterName)
	mc := &unstructured.Unstructured{}
	mc.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	mc.SetName(clusterName)
	ssh := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-site-ansible-ssh", Namespace: ns},
		Data:       map[string][]byte{"id_rsa": []byte("test-key")},
	}
	r, c := newDeleteTestReconciler(t, fcj, mc, ssh)

	if _, err := r.reconcileClusterDelete(context.Background(), fcj, clusterName); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got fleetv1alpha1.FleetClusterJoin
	if err := c.Get(context.Background(), types.NamespacedName{Name: clusterName, Namespace: ns}, &got); err != nil {
		t.Fatal(err)
	}
	if deleteJobSucceeded(&got) {
		t.Fatal("expected delete-job-succeeded cleared for retry")
	}
	if got.Status.DeleteJobName != "fleet-cluster-delete-"+clusterName {
		t.Fatalf("delete job name=%q", got.Status.DeleteJobName)
	}
	var job batchv1.Job
	if err := c.Get(context.Background(), types.NamespacedName{Name: got.Status.DeleteJobName, Namespace: ns}, &job); err != nil {
		t.Fatalf("delete job: %v", err)
	}
	if done, _ := jobIsFinished(&job); done {
		t.Fatal("retry should start a new delete job")
	}
}
