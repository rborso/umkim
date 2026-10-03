package storage

import (
	"reflect"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

// SpecEqual reports whether two storage specs are equivalent for idempotent apply.
func SpecEqual(a, b *fleetv1alpha1.ClusterStorageSpec) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return reflect.DeepEqual(normalizeStorageSpec(a), normalizeStorageSpec(b))
}

func normalizeStorageSpec(s *fleetv1alpha1.ClusterStorageSpec) fleetv1alpha1.ClusterStorageSpec {
	out := *s
	if out.Backend == "" {
		out.Backend = fleetv1alpha1.StorageBackendLocalPath
	}
	if out.NFS != nil {
		nfs := *out.NFS
		out.NFS = &nfs
	}
	if out.Longhorn != nil {
		lh := *out.Longhorn
		out.Longhorn = &lh
	}
	return out
}
