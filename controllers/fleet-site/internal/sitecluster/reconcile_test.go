package sitecluster

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitehosts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileSiteSyncsReadyFCJOverWaitingGroup(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fleetv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "homelab",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
		},
	}
	fcj := &fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{Name: "lab-ha-1", Namespace: ns},
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			Phase:   fleetv1alpha1.JoinPhaseReady,
			Message: "Capability discovery running",
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(site, fcj).Build()
	cfg := config.Config{Namespace: ns}

	intent := &fleetv1alpha1.SiteHostClusterIntent{
		Mode:            fleetv1alpha1.SiteHostClusterModeEnroll,
		ClusterName:     "lab-ha-1",
		Topology:        fleetv1alpha1.ClusterTopologyHA3Node,
		HAGroup:         "lab-ha-1",
		ControlPlaneVip: "203.0.113.1",
	}
	for _, name := range []string{"lab-ha-1-cp0", "lab-ha-1-cp1", "lab-ha-1-cp2"} {
		rec := fleetv1alpha1.SiteHostRecord{
			Name:          name,
			Kind:          fleetv1alpha1.SiteHostKindVM,
			AnsibleHost:   "203.0.113.10",
			ClusterIntent: intent,
			Status: fleetv1alpha1.SiteHostStatus{
				Phase: fleetv1alpha1.SiteHostPhaseReady,
				Cluster: &fleetv1alpha1.SiteHostClusterStatus{
					Mode:        fleetv1alpha1.SiteHostClusterModeEnroll,
					ClusterName: "lab-ha-1",
					FCJName:     "lab-ha-1",
					Phase:       fleetv1alpha1.SiteHostClusterPhaseWaitingGroup,
					Message:     "waiting for HA group leader to create cluster",
				},
			},
		}
		if err := sitehosts.Put(context.Background(), c, ns, "homelab", rec); err != nil {
			t.Fatal(err)
		}
	}

	if err := ReconcileSite(context.Background(), c, cfg, "homelab"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"lab-ha-1-cp0", "lab-ha-1-cp1", "lab-ha-1-cp2"} {
		updated, ok, err := sitehosts.Get(context.Background(), c, ns, "homelab", name)
		if err != nil || !ok {
			t.Fatal(err)
		}
		if updated.Status.Cluster == nil || updated.Status.Cluster.Phase != fleetv1alpha1.SiteHostClusterPhaseReady {
			t.Fatalf("%s cluster status=%+v want Ready", name, updated.Status.Cluster)
		}
	}
}
