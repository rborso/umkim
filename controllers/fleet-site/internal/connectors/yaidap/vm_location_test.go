package yaidap

import (
	"context"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func seedVmLocationInventoryCM(ns, site string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName(site, fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: ns,
		},
		Data: map[string]string{
			connectors.FileKey("guests.yml"): `guests:
  hosts:
    vm01:
      ansible_host: 203.0.113.10
      vm_location: least-utilized
external:
  hosts: {}
`,
			connectors.FileKey("host_vars/vm01.yml"): `---
ansible_host: 203.0.113.10
vm_location: least-utilized
`,
			connectors.FileKey("hypervisors.yml"): `hypervisors:
  hosts:
    virt02:
      ansible_host: 172.16.10.2
`,
			"_meta.meta.json": `{"syncPhase":"synced"}`,
		},
	}
}

func TestPersistResolvedVmLocation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	ns := "fleet-console"
	site := "homelab"
	cm := seedVmLocationInventoryCM(ns, site)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	changed, err := PersistResolvedVmLocation(context.Background(), c, ns, site, "vm01", "virt02", "system/test")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected inventory change")
	}

	var updated corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Namespace: ns,
		Name:      connectors.InventoryCMName(site, fleetv1alpha1.ProvisionProviderYaidap),
	}, &updated); err != nil {
		t.Fatal(err)
	}
	guests := updated.Data[connectors.FileKey("guests.yml")]
	if !strings.Contains(guests, "vm_location: virt02") {
		t.Fatalf("guests.yml=%q", guests)
	}
	hostVars := updated.Data[connectors.FileKey("host_vars/vm01.yml")]
	if !strings.Contains(hostVars, "vm_location: virt02") {
		t.Fatalf("host_vars=%q", hostVars)
	}
	metaRaw := updated.Data["_meta.meta.json"]
	if !strings.Contains(metaRaw, `"syncPhase":"dirty"`) && !strings.Contains(metaRaw, `"syncPhase": "dirty"`) {
		t.Fatalf("meta=%q", metaRaw)
	}
}

func TestPersistResolvedVmLocationRejectsScheduleToken(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	ns := "fleet-console"
	site := "homelab"
	cm := seedVmLocationInventoryCM(ns, site)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	_, err := PersistResolvedVmLocation(context.Background(), c, ns, site, "vm01", "least-utilized", "system/test")
	if err == nil {
		t.Fatal("expected error for schedule token")
	}
}

func TestPersistResolvedVmLocationRejectsUnknownHypervisor(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	ns := "fleet-console"
	site := "homelab"
	cm := seedVmLocationInventoryCM(ns, site)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	_, err := PersistResolvedVmLocation(context.Background(), c, ns, site, "vm01", "virt99", "system/test")
	if err == nil {
		t.Fatal("expected error for unknown hypervisor")
	}
}
