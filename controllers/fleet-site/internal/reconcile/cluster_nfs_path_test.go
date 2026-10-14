package reconcile

import (
	"strings"
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// A rebuilt cluster is a new object, so it must get a fresh export directory;
// within a cluster's life the path is stable.
func TestNFSClusterPathIsUniquePerClusterObject(t *testing.T) {
	first := &fleetv1alpha1.FleetClusterJoin{ObjectMeta: metav1.ObjectMeta{Name: "c1", UID: types.UID("11111111-aaaa-bbbb-cccc-dddddddddddd")}}
	rebuilt := &fleetv1alpha1.FleetClusterJoin{ObjectMeta: metav1.ObjectMeta{Name: "c1", UID: types.UID("22222222-aaaa-bbbb-cccc-dddddddddddd")}}
	a := nfsClusterPath(first, "c1")
	b := nfsClusterPath(rebuilt, "c1")
	if a == b {
		t.Fatalf("rebuilt cluster reused the export path %q", a)
	}
	if !strings.HasPrefix(a, "c1-") || !strings.HasPrefix(b, "c1-") {
		t.Fatalf("paths should stay recognisable: %q %q", a, b)
	}
	if nfsClusterPath(first, "c1") != a {
		t.Fatal("path must be stable for the same cluster object")
	}
}

func TestNFSClusterPathFallsBackWithoutUID(t *testing.T) {
	// Unit tests and hand-built objects have no UID.
	if got := nfsClusterPath(&fleetv1alpha1.FleetClusterJoin{}, "c1"); got != "c1" {
		t.Fatalf("path = %q, want c1", got)
	}
}
