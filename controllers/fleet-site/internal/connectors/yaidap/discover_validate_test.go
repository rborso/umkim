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

func TestValidateDiscoverPXEActionBareMetal(t *testing.T) {
	site := "homelab"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName(site, fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: "fleet-console",
		},
		Data: map[string]string{
			"file.bare_metals.yml": `bare_metals:
  hosts:
    gpu-bm-1:
      ansible_host: 172.16.10.6
`,
			"file.hypervisors.yml": `hypervisors:
  hosts: {}
`,
			"file.host_vars/gpu-bm-1.yml": `---
tempxe_discovery:
  enabled: true
uplinks:
  - name: n0p0
    mac_address: "52:54:00:10:07:01"
`,
		},
	}
	c := fake.NewClientBuilder().WithObjects(cm).Build()
	if err := ValidateDiscoverPXEAction(context.Background(), c, "fleet-console", site, "gpu-bm-1"); err != nil {
		t.Fatalf("expected bare_metal discover_pxe to pass validation: %v", err)
	}
}

func TestValidateDiscoverPXEActionRequiresIndex(t *testing.T) {
	site := "homelab"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName(site, fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: "fleet-console",
		},
		Data: map[string]string{
			"file.bare_metals.yml": `bare_metals:
  hosts: {}
`,
			"file.hypervisors.yml": `hypervisors:
  hosts: {}
`,
			"file.host_vars/orphan.yml": `---
tempxe_discovery:
  enabled: true
uplinks:
  - mac_address: "52:54:00:10:07:01"
`,
		},
	}
	c := fake.NewClientBuilder().WithObjects(cm).Build()
	err := ValidateDiscoverPXEAction(context.Background(), c, "fleet-console", site, "orphan")
	if err == nil {
		t.Fatal("expected error when host not in hypervisors or bare_metals")
	}
}
