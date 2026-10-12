package yaidap

import (
	"context"
	"fmt"
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	guestsStale = `guests:
  hosts:
    vm01:
      ansible_host: 203.0.113.10
      vm_location: least-utilized
external:
  hosts:
    bm01:
      ansible_host: 203.0.113.20
`
	guestsResolved = `guests:
  hosts:
    vm01:
      ansible_host: 203.0.113.10
      vm_location: virt02
external:
  hosts:
    bm01:
      ansible_host: 203.0.113.20
`
)

func migrateLegacyStoreCM(ns, site string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectors.InventoryCMName(site, fleetv1alpha1.ProvisionProviderYaidap),
			Namespace: ns,
		},
		Data: map[string]string{
			connectors.FileKey("guests.yml"): guestsStale,
			connectors.FileKey("bare_metals.yml"): `bare_metals:
  hosts:
    bm01:
      ansible_host: 203.0.113.20
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

func readGuests(t *testing.T, c client.Client, ns, site string) string {
	t.Helper()
	var cm corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Namespace: ns,
		Name:      connectors.InventoryCMName(site, fleetv1alpha1.ProvisionProviderYaidap),
	}, &cm); err != nil {
		t.Fatal(err)
	}
	return cm.Data[connectors.FileKey("guests.yml")]
}

// A create_vm that finishes while MigrateLegacyStore is running persists a resolved
// vm_location. Because MigrateLegacyStore runs before every provision action
// (EnsureReadyForProvision), it must not revert that write from an earlier snapshot.
func TestMigrateLegacyStoreDoesNotClobberResolvedVmLocation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	ns, site := "fleet-console", "homelab"
	invName := connectors.InventoryCMName(site, fleetv1alpha1.ProvisionProviderYaidap)

	injected := false
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(migrateLegacyStoreCM(ns, site)).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				cmObj, ok := obj.(*corev1.ConfigMap)
				if ok && cmObj.Name == invName && !injected {
					injected = true
					// Simulate the concurrent create_vm persist, then fail this write so
					// the store (not the caller's snapshot) is what the retry observes.
					var cur corev1.ConfigMap
					if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: invName}, &cur); err != nil {
						return err
					}
					if cur.Data == nil {
						cur.Data = map[string]string{}
					}
					cur.Data[connectors.FileKey("guests.yml")] = guestsResolved
					if err := cl.Update(ctx, &cur); err != nil {
						return err
					}
					return apierrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, invName, fmt.Errorf("simulated concurrent write"))
				}
				return cl.Update(ctx, obj, opts...)
			},
		}).
		Build()

	if err := MigrateLegacyStore(context.Background(), c, ns, site, "system/test"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !injected {
		t.Fatal("test did not exercise the concurrent-write path")
	}
	got := readGuests(t, c, ns, site)
	if !strings.Contains(got, "vm_location: virt02") {
		t.Fatalf("resolved vm_location was clobbered:\n%s", got)
	}
}
