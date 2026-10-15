package ingressauth

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A cluster with no access rules and no OIDC clients still needs its own
// ManifestWork, so adding one must change the Ansible spec hash. Without this the
// controller saw "nothing changed", never re-ran the reconcile, and could report
// the cluster ready with no spoke stack.
func TestAnsibleSpecHashTracksEnabledClusters(t *testing.T) {
	fia := &fleetv1alpha1.FleetIngressAuth{
		ObjectMeta: metav1.ObjectMeta{Name: fleetv1alpha1.FleetIngressAuthName},
		Spec:       fleetv1alpha1.FleetIngressAuthSpec{HubEnabled: true},
	}
	one := DesiredState{EnabledClusters: []string{"cluster-a"}}
	two := DesiredState{EnabledClusters: []string{"cluster-a", "cluster-b"}}

	if AnsibleSpecHash(fia, one) == AnsibleSpecHash(fia, two) {
		t.Fatal("hash did not change when an enabled cluster was added")
	}
	// Order must not matter.
	reordered := DesiredState{EnabledClusters: []string{"cluster-b", "cluster-a"}}
	if AnsibleSpecHash(fia, two) != AnsibleSpecHash(fia, reordered) {
		t.Fatal("hash changed with cluster order")
	}
}

func TestAnsibleSpecChangedAfterClusterAdded(t *testing.T) {
	fia := &fleetv1alpha1.FleetIngressAuth{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fleetv1alpha1.FleetIngressAuthName,
			Annotations: map[string]string{},
		},
		Spec: fleetv1alpha1.FleetIngressAuthSpec{HubEnabled: true},
	}
	one := DesiredState{EnabledClusters: []string{"cluster-a"}}
	// What a completed reconcile stores when it rendered only cluster-a.
	fia.Annotations[AnsibleSpecHashAnnotation] = AnsibleSpecHash(fia, one)

	if AnsibleSpecChanged(fia, one) {
		t.Fatal("unchanged desired state reported as changed")
	}
	two := DesiredState{EnabledClusters: []string{"cluster-a", "cluster-b"}}
	if !AnsibleSpecChanged(fia, two) {
		t.Fatal("cluster added after the reconcile was not detected")
	}
}
