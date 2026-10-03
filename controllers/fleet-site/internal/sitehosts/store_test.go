package sitehosts

import (
	"context"
	"fmt"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestUpdateClusterStatusRetriesOnConflict(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	ns := "fleet-console"
	site := "homelab"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cmName(site),
			Namespace:       ns,
			ResourceVersion: "1",
		},
		Data: map[string]string{
			"hosts.json": `[{"name":"lab-ha-1-cp2","kind":"vm","status":{"phase":"Ready","cluster":{"phase":"Failed","clusterName":"lab-ha-1"}}}]`,
		},
	}

	updates := 0
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				updates++
				if updates == 1 {
					return apierrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, cm.Name, fmt.Errorf("modified"))
				}
				return cl.Update(ctx, obj, opts...)
			},
		}).
		Build()

	err := UpdateClusterStatus(context.Background(), c, ns, site, "lab-ha-1-cp2", fleetv1alpha1.SiteHostClusterStatus{
		Phase:       fleetv1alpha1.SiteHostClusterPhaseReady,
		Message:     "Capability discovery running",
		ClusterName: "lab-ha-1",
		FCJName:     "lab-ha-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updates < 2 {
		t.Fatalf("expected retry on conflict, updates=%d", updates)
	}

	hosts, err := List(context.Background(), c, ns, site)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Status.Cluster == nil || hosts[0].Status.Cluster.Phase != fleetv1alpha1.SiteHostClusterPhaseReady {
		t.Fatalf("cluster status=%+v", hosts)
	}
}
