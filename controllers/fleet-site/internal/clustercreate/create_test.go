package clustercreate

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreateStampsEffectiveSiteID(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName: "homelab",
			SiteId:   "ho",
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(site).Build()
	fcj, err := Create(context.Background(), c, config.Config{Namespace: ns}, Request{
		ClusterName:   "lab",
		SiteId:        "ho",
		BootstrapHost: "198.51.100.20",
		Nodes: []fleetv1alpha1.FleetClusterJoinNodeSpec{{
			Hostname: "vm01",
			Address:  "198.51.100.20",
			Role:     fleetv1alpha1.NodeRoleControl,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fcj.Spec.SiteId != "ho" {
		t.Fatalf("siteId=%q", fcj.Spec.SiteId)
	}
}
