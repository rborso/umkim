package storage

import (
	"encoding/json"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
)

const NFSValidateJSONPrefix = "UMKIM_NFS_VALIDATE_JSON:"

// NFSValidateCheck is one row in a storage validation report.
type NFSValidateCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// NFSValidateResult is parsed from ansible-runner job logs.
type NFSValidateResult struct {
	OK         bool               `json:"ok"`
	Checks     []NFSValidateCheck `json:"checks"`
	NFSPath    string             `json:"nfsPath,omitempty"`
	MountPath  string             `json:"mountPath,omitempty"`
	Node       string             `json:"node,omitempty"`
}

// ValidateNFSSpecProbe checks NFS form fields for a read-only connectivity probe.
func ValidateNFSSpecProbe(st *fleetv1alpha1.ClusterStorageSpec, fcj *fleetv1alpha1.FleetClusterJoin) error {
	return validateNFSSpecFields(st, fcj)
}

// NFSSpecExtraArgs returns ansible -e overrides for a one-off NFS validate job.
func NFSSpecExtraArgs(st *fleetv1alpha1.ClusterStorageSpec, clusterName string) []string {
	if st == nil || st.NFS == nil {
		return nil
	}
	nfs := st.NFS
	exportBase := strings.TrimSpace(nfs.ExportBase)
	if exportBase == "" {
		exportBase = "/srv/fleet-nfs"
	}
	clusterPath := strings.TrimSpace(nfs.ClusterPath)
	if clusterPath == "" {
		clusterPath = clusterName
	}
	args := []string{
		"-e", "fleet_cluster_storage_backend=nfs_external",
		"-e", "cluster_storage_nfs_server=" + strings.TrimSpace(nfs.Server),
		"-e", "cluster_storage_nfs_export_base=" + exportBase,
		"-e", "cluster_storage_nfs_cluster_path=" + clusterPath,
	}
	if sc := strings.TrimSpace(nfs.StorageClass); sc != "" {
		args = append(args, "-e", "cluster_storage_nfs_storage_class="+sc)
	}
	if delegate := strings.TrimSpace(nfs.DelegateHost); delegate != "" {
		args = append(args, "-e", "cluster_storage_nfs_delegate_host="+delegate)
	} else {
		// Override stale inventory delegate from FleetClusterJoin spec.
		args = append(args, "-e", "fleet_cluster_nfs_delegate_host=")
		args = append(args, "-e", "cluster_storage_nfs_delegate_host=")
	}
	if mountPath := strings.TrimSpace(nfs.MountPath); mountPath != "" {
		args = append(args, "-e", "cluster_storage_nfs_mount_path="+mountPath)
	}
	return args
}

// ParseNFSValidateJobLogs extracts the JSON marker emitted by nfs_validate.yml.
func ParseNFSValidateJobLogs(logs string) (*NFSValidateResult, bool) {
	for _, line := range strings.Split(logs, "\n") {
		idx := strings.Index(line, NFSValidateJSONPrefix)
		if idx < 0 {
			continue
		}
		rest := line[idx+len(NFSValidateJSONPrefix):]
		start := strings.Index(rest, "{")
		end := strings.LastIndex(rest, "}")
		if start < 0 || end <= start {
			continue
		}
		raw := rest[start : end+1]
		raw = strings.ReplaceAll(raw, `\"`, `"`)
		raw = strings.Trim(raw, `"' `)
		var res NFSValidateResult
		if err := json.Unmarshal([]byte(raw), &res); err != nil {
			continue
		}
		return &res, true
	}
	return nil, false
}
