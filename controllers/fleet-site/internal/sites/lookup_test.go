package sites

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFindFleetSiteBySiteID(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := fleetv1alpha1.AddToScheme(scheme); err != nil {
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

	got, err := FindFleetSite(context.Background(), c, ns, "ho")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "homelab" {
		t.Fatalf("name=%q", got.Name)
	}
	if got.Spec.EffectiveSiteID(got.Name) != "ho" {
		t.Fatalf("effective id=%q", got.Spec.EffectiveSiteID(got.Name))
	}
}
