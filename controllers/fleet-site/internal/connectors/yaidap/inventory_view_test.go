package yaidap

import (
	"context"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestListInventoryHostsIncludesHypervisors(t *testing.T) {
	site := "homelab"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName(site, fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: "fleet-console",
		},
		Data: map[string]string{
			"file.guests.yml": `guests:
  hosts:
    lab-single-1-0:
      vm_location: virt02
      ansible_host: 192.168.1.101
`,
			"file.hypervisors.yml": `hypervisors:
  hosts:
    virt03:
      ansible_host: 172.16.10.4
`,
			"_meta.status.json": `{}`,
		},
	}
	c := fake.NewClientBuilder().WithObjects(cm).Build()
	hosts, err := ListInventoryHosts(context.Background(), c, "fleet-console", site)
	if err != nil {
		t.Fatal(err)
	}
	var virt03 *InventoryHostView
	for i := range hosts {
		if hosts[i].Name == "virt03" {
			virt03 = &hosts[i]
			break
		}
	}
	if virt03 == nil {
		t.Fatal("expected virt03 in host list")
	}
	if virt03.Kind != fleetv1alpha1.SiteHostKindHypervisor {
		t.Fatalf("expected hypervisor kind, got %q", virt03.Kind)
	}
	if virt03.AnsibleHost != "172.16.10.4" {
		t.Fatalf("unexpected ansible_host %q", virt03.AnsibleHost)
	}
}

func TestListInventoryHostsExternalBareMetal(t *testing.T) {
	site := "homelab"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName(site, fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: "fleet-console",
		},
		Data: map[string]string{
			"file.guests.yml": `guests:
  hosts: {}
`,
			"file.bare_metals.yml": `bare_metals:
  hosts:
    bm01:
      ansible_host: 203.0.113.11
`,
			"file.hypervisors.yml": `hypervisors:
  hosts: {}
`,
			"_meta.status.json": `{}`,
		},
	}
	c := fake.NewClientBuilder().WithObjects(cm).Build()
	hosts, err := ListInventoryHosts(context.Background(), c, "fleet-console", site)
	if err != nil {
		t.Fatal(err)
	}
	var bm01 *InventoryHostView
	for i := range hosts {
		if hosts[i].Name == "bm01" {
			bm01 = &hosts[i]
			break
		}
	}
	if bm01 == nil {
		t.Fatal("expected bm01 in host list")
	}
	if bm01.Kind != fleetv1alpha1.SiteHostKindBareMetal {
		t.Fatalf("expected bare_metal kind, got %q", bm01.Kind)
	}
	if !bm01.External {
		t.Fatal("expected external=true")
	}
}
