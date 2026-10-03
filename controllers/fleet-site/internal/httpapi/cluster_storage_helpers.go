package httpapi

import fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"

type clusterStorageBody struct {
	LocalPath     *struct {
		Enabled *bool `json:"enabled,omitempty"`
	} `json:"localPath,omitempty"`
	Backend       string `json:"backend,omitempty"`
	MinDataDiskGi int    `json:"minDataDiskGi,omitempty"`
	Rwo           bool   `json:"rwo,omitempty"`
	Rwx           bool   `json:"rwx,omitempty"`
	NFS           *struct {
		Enabled      bool   `json:"enabled,omitempty"`
		Server       string `json:"server,omitempty"`
		ExportBase   string `json:"exportBase,omitempty"`
		ClusterPath  string `json:"clusterPath,omitempty"`
		StorageClass string `json:"storageClass,omitempty"`
		DelegateHost string `json:"delegateHost,omitempty"`
		MountPath    string `json:"mountPath,omitempty"`
	} `json:"nfs,omitempty"`
	Longhorn *struct {
		Enabled      bool   `json:"enabled,omitempty"`
		Device       string `json:"device,omitempty"`
		StorageClass string `json:"storageClass,omitempty"`
	} `json:"longhorn,omitempty"`
	Force bool `json:"force,omitempty"`
}

func storageSpecFromCreateBody(body *struct {
	LocalPath     *struct {
		Enabled *bool `json:"enabled,omitempty"`
	} `json:"localPath,omitempty"`
	Backend       string `json:"backend,omitempty"`
	MinDataDiskGi int    `json:"minDataDiskGi,omitempty"`
	Rwo           bool   `json:"rwo,omitempty"`
	Rwx           bool   `json:"rwx,omitempty"`
	NFS           *struct {
		Enabled      bool   `json:"enabled,omitempty"`
		Server       string `json:"server,omitempty"`
		ExportBase   string `json:"exportBase,omitempty"`
		ClusterPath  string `json:"clusterPath,omitempty"`
		StorageClass string `json:"storageClass,omitempty"`
		DelegateHost string `json:"delegateHost,omitempty"`
		MountPath    string `json:"mountPath,omitempty"`
	} `json:"nfs,omitempty"`
	Longhorn *struct {
		Enabled      bool   `json:"enabled,omitempty"`
		Device       string `json:"device,omitempty"`
		StorageClass string `json:"storageClass,omitempty"`
	} `json:"longhorn,omitempty"`
}) *fleetv1alpha1.ClusterStorageSpec {
	if body == nil {
		return nil
	}
	return storageSpecFromBody(&clusterStorageBody{
		LocalPath:     body.LocalPath,
		Backend:       body.Backend,
		MinDataDiskGi: body.MinDataDiskGi,
		Rwo:           body.Rwo,
		Rwx:           body.Rwx,
		NFS:           body.NFS,
		Longhorn:      body.Longhorn,
	})
}

func storageSpecFromBody(body *clusterStorageBody) *fleetv1alpha1.ClusterStorageSpec {
	if body == nil {
		return nil
	}
	st := &fleetv1alpha1.ClusterStorageSpec{
		Backend:       body.Backend,
		MinDataDiskGi: body.MinDataDiskGi,
		Rwo:           body.Rwo,
		Rwx:           body.Rwx,
	}
	if body.LocalPath != nil {
		st.LocalPath = &fleetv1alpha1.LocalPathStorageSpec{Enabled: body.LocalPath.Enabled}
	}
	if body.NFS != nil {
		st.NFS = &fleetv1alpha1.ClusterNFSSpec{
			Enabled:      body.NFS.Enabled || body.Backend == fleetv1alpha1.StorageBackendNFSExternal,
			Server:       body.NFS.Server,
			ExportBase:   body.NFS.ExportBase,
			ClusterPath:  body.NFS.ClusterPath,
			StorageClass: body.NFS.StorageClass,
			DelegateHost: body.NFS.DelegateHost,
			MountPath:    body.NFS.MountPath,
		}
	} else if body.Backend == fleetv1alpha1.StorageBackendNFSExternal {
		st.NFS = &fleetv1alpha1.ClusterNFSSpec{Enabled: true}
	} else if body.Backend == fleetv1alpha1.StorageBackendNFSLocal {
		st.NFS = &fleetv1alpha1.ClusterNFSSpec{
			ExportBase:   "/srv/fleet-nfs",
			StorageClass: "nfs-client",
		}
		st.Rwx = true
		if !st.Rwo {
			st.Rwo = true
		}
	}
	if body.Longhorn != nil {
		st.Longhorn = &fleetv1alpha1.ClusterLonghornSpec{
			Enabled:      body.Longhorn.Enabled || body.Backend == fleetv1alpha1.StorageBackendLonghorn,
			Device:       body.Longhorn.Device,
			StorageClass: body.Longhorn.StorageClass,
		}
	} else if body.Backend == fleetv1alpha1.StorageBackendLonghorn {
		st.Longhorn = &fleetv1alpha1.ClusterLonghornSpec{Enabled: true}
	}
	if st.LonghornEnabled() && (st.LocalPath == nil || st.LocalPath.Enabled == nil) {
		disabled := false
		st.LocalPath = &fleetv1alpha1.LocalPathStorageSpec{Enabled: &disabled}
	}
	return st
}
