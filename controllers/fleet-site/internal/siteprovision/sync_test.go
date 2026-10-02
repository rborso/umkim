package siteprovision

import (
	"context"
	"encoding/json"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRefreshRunningJobSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fleetv1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "homelab",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
		},
		Status: fleetv1alpha1.FleetSiteStatus{
			Provision: &fleetv1alpha1.SiteProvisionStatus{
				Current: &fleetv1alpha1.SiteProvisionActionRecord{
					ID:       "abc123",
					Action:   "create_vm",
					Subject:  "vm01",
					Phase:    fleetv1alpha1.ProvisionPhaseRunning,
					JobName:  "fleet-site-prov-abc123",
					Provider: fleetv1alpha1.ProvisionProviderYaidap,
				},
			},
		},
	}
	invCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: ns,
		},
		Data: map[string]string{
			"file.guests.yml": `guests:
  hosts:
    vm01:
      ansible_host: 203.0.113.10
`,
			"_meta.status.json": `{"guests":{"vm01":{"phase":"Provisioning"}}}`,
		},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-site-prov-abc123", Namespace: ns},
		Status:     batchv1.JobStatus{Succeeded: 1},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&fleetv1alpha1.FleetSite{}).WithObjects(site, invCM, job).Build()

	if err := RefreshRunningJob(context.Background(), c, ns, site, nil, nil); err != nil {
		t.Fatal(err)
	}
	if site.Status.Provision.Current != nil {
		t.Fatalf("expected current cleared, got %+v", site.Status.Provision.Current)
	}
	if len(site.Status.Provision.History) != 1 || site.Status.Provision.History[0].Phase != fleetv1alpha1.ProvisionPhaseSucceeded {
		t.Fatalf("history=%+v", site.Status.Provision.History)
	}

	var updatedCM corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Namespace: ns,
		Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
	}, &updatedCM); err != nil {
		t.Fatal(err)
	}
	var status struct {
		Guests map[string]struct {
			Phase string `json:"phase"`
		} `json:"guests"`
	}
	if err := json.Unmarshal([]byte(updatedCM.Data["_meta.status.json"]), &status); err != nil {
		t.Fatal(err)
	}
	if status.Guests["vm01"].Phase != fleetv1alpha1.SiteHostPhaseReady {
		t.Fatalf("status=%+v", status.Guests["vm01"])
	}
}

func TestRepairStaleGuestProvisioning(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fleetv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Status: fleetv1alpha1.FleetSiteStatus{
			Provision: &fleetv1alpha1.SiteProvisionStatus{
				History: []fleetv1alpha1.SiteProvisionActionRecord{{
					ID:      "5894781b6b07",
					Action:  "create_vm",
					Subject: "lab-ha-0-cp1",
					Phase:   fleetv1alpha1.ProvisionPhaseSucceeded,
					Message: "YAIDAP connector job succeeded",
					Host:    &fleetv1alpha1.ProvisionHandoffHost{AnsibleHost: "172.16.10.101"},
				}},
			},
		},
	}
	invCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: ns,
		},
		Data: map[string]string{
			"_meta.status.json": `{"guests":{"lab-ha-0-cp1":{"phase":"Provisioning","message":"vm_location=virt01","vm_location":"virt01"}}}`,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&fleetv1alpha1.FleetSite{}).WithObjects(site, invCM).Build()

	RepairStaleGuestProvisioning(context.Background(), c, ns, site)

	var updatedCM corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Namespace: ns,
		Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
	}, &updatedCM); err != nil {
		t.Fatal(err)
	}
	var status struct {
		Guests map[string]struct {
			Phase   string `json:"phase"`
			Message string `json:"message"`
		} `json:"guests"`
	}
	if err := json.Unmarshal([]byte(updatedCM.Data["_meta.status.json"]), &status); err != nil {
		t.Fatal(err)
	}
	if status.Guests["lab-ha-0-cp1"].Phase != fleetv1alpha1.SiteHostPhaseReady {
		t.Fatalf("status=%+v", status.Guests["lab-ha-0-cp1"])
	}
}

func TestRefreshRunningJobDestroySuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fleetv1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Spec: fleetv1alpha1.FleetSiteSpec{
			SiteName:         "homelab",
			SiteId:           "homelab",
			ProvisioningMode: fleetv1alpha1.ProvisioningModeRegistry,
		},
		Status: fleetv1alpha1.FleetSiteStatus{
			Provision: &fleetv1alpha1.SiteProvisionStatus{
				Current: &fleetv1alpha1.SiteProvisionActionRecord{
					ID:       "def456",
					Action:   fleetv1alpha1.ProvisionActionDestroy,
					Subject:  "vm01",
					Phase:    fleetv1alpha1.ProvisionPhaseRunning,
					JobName:  "fleet-site-prov-def456",
					Provider: fleetv1alpha1.ProvisionProviderYaidap,
				},
			},
		},
	}
	invCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: ns,
		},
		Data: map[string]string{
			"file.guests.yml": `guests:
  hosts:
    vm01:
      ansible_host: 203.0.113.10
`,
			"_meta.status.json": `{"guests":{"vm01":{"phase":"Provisioning","cluster":{"clusterName":"lab-ha-0","phase":"Ready"}}}}`,
		},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-site-prov-def456", Namespace: ns},
		Status:     batchv1.JobStatus{Succeeded: 1},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&fleetv1alpha1.FleetSite{}).WithObjects(site, invCM, job).Build()

	if err := RefreshRunningJob(context.Background(), c, ns, site, nil, nil); err != nil {
		t.Fatal(err)
	}

	var updatedCM corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Namespace: ns,
		Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
	}, &updatedCM); err != nil {
		t.Fatal(err)
	}
	var status struct {
		Guests map[string]struct {
			Phase   string `json:"phase"`
			Message string `json:"message"`
			Cluster *struct {
				ClusterName string `json:"clusterName"`
			} `json:"cluster"`
		} `json:"guests"`
	}
	if err := json.Unmarshal([]byte(updatedCM.Data["_meta.status.json"]), &status); err != nil {
		t.Fatal(err)
	}
	g := status.Guests["vm01"]
	if g.Phase != fleetv1alpha1.SiteHostPhaseDeclared {
		t.Fatalf("phase=%q want Declared", g.Phase)
	}
	if g.Message == "" {
		t.Fatalf("expected destroy success message, got %+v", g)
	}
	if g.Cluster != nil && g.Cluster.ClusterName != "" {
		t.Fatalf("cluster should be cleared, got %+v", g.Cluster)
	}
}

func TestSyncHostInventoryDestroyRunning(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fleetv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	ns := "fleet-console"
	invCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: ns,
		},
		Data: map[string]string{
			"_meta.status.json": `{"guests":{"vm01":{"phase":"Ready"}}}`,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(invCM).Build()

	rec := fleetv1alpha1.SiteProvisionActionRecord{
		ID:      "run1",
		Action:  fleetv1alpha1.ProvisionActionDestroy,
		Subject: "vm01",
		Phase:   fleetv1alpha1.ProvisionPhaseRunning,
	}
	SyncHostInventory(context.Background(), c, ns, "homelab", rec)

	var updatedCM corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Namespace: ns,
		Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
	}, &updatedCM); err != nil {
		t.Fatal(err)
	}
	var status struct {
		Guests map[string]struct {
			Phase string `json:"phase"`
		} `json:"guests"`
	}
	if err := json.Unmarshal([]byte(updatedCM.Data["_meta.status.json"]), &status); err != nil {
		t.Fatal(err)
	}
	if status.Guests["vm01"].Phase != fleetv1alpha1.SiteHostPhaseDestroying {
		t.Fatalf("phase=%q want Destroying", status.Guests["vm01"].Phase)
	}
}

func TestRepairStaleGuestProvisioningDestroy(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fleetv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	ns := "fleet-console"
	site := &fleetv1alpha1.FleetSite{
		ObjectMeta: metav1.ObjectMeta{Name: "homelab", Namespace: ns},
		Status: fleetv1alpha1.FleetSiteStatus{
			Provision: &fleetv1alpha1.SiteProvisionStatus{
				History: []fleetv1alpha1.SiteProvisionActionRecord{{
					ID:      "destroy123",
					Action:  fleetv1alpha1.ProvisionActionDestroy,
					Subject: "vm01",
					Phase:   fleetv1alpha1.ProvisionPhaseSucceeded,
					Message: "YAIDAP connector job succeeded",
				}},
			},
		},
	}
	invCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: ns,
		},
		Data: map[string]string{
			"_meta.status.json": `{"guests":{"vm01":{"phase":"Provisioning"}}}`,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&fleetv1alpha1.FleetSite{}).WithObjects(site, invCM).Build()

	RepairStaleGuestProvisioning(context.Background(), c, ns, site)

	var updatedCM corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Namespace: ns,
		Name:      connectors.InventoryCMName("homelab", fleetv1alpha1.ProvisionProviderYaidap),
	}, &updatedCM); err != nil {
		t.Fatal(err)
	}
	var status struct {
		Guests map[string]struct {
			Phase string `json:"phase"`
		} `json:"guests"`
	}
	if err := json.Unmarshal([]byte(updatedCM.Data["_meta.status.json"]), &status); err != nil {
		t.Fatal(err)
	}
	if status.Guests["vm01"].Phase != fleetv1alpha1.SiteHostPhaseDeclared {
		t.Fatalf("status=%+v", status.Guests["vm01"])
	}
}
