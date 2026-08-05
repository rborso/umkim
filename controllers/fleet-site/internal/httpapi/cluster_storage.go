package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/storage"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	configureStorageAnnotation    = "fleet.umkim.com/configure-storage"
	configureStorageJobAnnotation = "fleet.umkim.com/configure-storage-job"
)

func (s *Server) handleConfigureStorage(ctx context.Context, w http.ResponseWriter, r *http.Request, clusterName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req clusterStorageBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Backend != "" &&
		req.Backend != fleetv1alpha1.StorageBackendNFSExternal &&
		req.Backend != fleetv1alpha1.StorageBackendNFSLocal {
		http.Error(w, "only nfs_external or nfs_local is supported for configure-storage", http.StatusBadRequest)
		return
	}
	next := storageSpecFromBody(&req)
	if next == nil || !next.NFSEnabled() {
		http.Error(w, "storage.nfs.enabled or backend nfs_local/nfs_external is required for configure-storage", http.StatusBadRequest)
		return
	}
	if req.Backend == fleetv1alpha1.StorageBackendNFSExternal && next.NFS != nil {
		next.NFS.Enabled = true
	}
	if req.Backend == fleetv1alpha1.StorageBackendNFSLocal {
		next.Backend = fleetv1alpha1.StorageBackendNFSLocal
		if next.NFS == nil {
			next.NFS = &fleetv1alpha1.ClusterNFSSpec{}
		}
		next.Rwx = true
		next.Rwo = true
	}
	if next.NFS == nil && !next.NFSLocalEnabled() {
		http.Error(w, "storage.nfs is required", http.StatusBadRequest)
		return
	}

	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster join not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		http.Error(w, "cluster must be Ready", http.StatusConflict)
		return
	}
	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if s.clusterReadOnly(clusterName, edge) {
		http.Error(w, "cluster is read-only", http.StatusForbidden)
		return
	}
	if fcj.Status.Storage.Phase == fleetv1alpha1.StoragePhaseApplying {
		http.Error(w, "storage configure already running", http.StatusConflict)
		return
	}
	if fcj.Annotations != nil {
		if _, ok := fcj.Annotations[configureStorageAnnotation]; ok {
			if fcj.Status.Storage.Phase != fleetv1alpha1.StoragePhaseFailed {
				http.Error(w, "storage configure already requested", http.StatusConflict)
				return
			}
			delete(fcj.Annotations, configureStorageAnnotation)
			delete(fcj.Annotations, configureStorageJobAnnotation)
		}
	}

	if next.NFS.ClusterPath == "" {
		next.NFS.ClusterPath = fcj.Spec.EffectiveClusterName(fcj.Name)
	}
	if err := storage.ValidateNFSSpec(next, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if storage.NFSSpecChanged(fcj.Spec.Storage, next, fcj.Spec.EffectiveClusterName(fcj.Name)) {
		http.Error(w, "changing NFS server or cluster export path is not supported; migrate workloads manually", http.StatusConflict)
		return
	}
	if storage.SpecEqual(fcj.Spec.Storage, next) &&
		fcj.Status.Storage.Phase == fleetv1alpha1.StoragePhaseApplied &&
		!req.Force {
		writeJSON(w, http.StatusAccepted, map[string]string{
			"phase":   fcj.Status.Storage.Phase,
			"message": "storage already applied",
		})
		return
	}

	fcj.Spec.Storage = next
	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	fcj.Annotations[configureStorageAnnotation] = time.Now().UTC().Format(time.RFC3339)
	if err := s.Client.Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fcj.Status.Storage.Phase = fleetv1alpha1.StoragePhasePending
	fcj.Status.Storage.Message = "Storage configure requested"
	fcj.Status.Message = fcj.Status.Storage.Message
	if err := s.Client.Status().Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"phase": fleetv1alpha1.StoragePhasePending})
}

func storageSummaryFromSpec(spec *fleetv1alpha1.ClusterStorageSpec) map[string]interface{} {
	if spec == nil {
		return nil
	}
	out := map[string]interface{}{
		"backend": spec.EffectiveBackend(),
		"rwo":     spec.Rwo,
		"rwx":     spec.Rwx,
		"localPath": map[string]interface{}{
			"enabled": spec.LocalPathEnabled(),
		},
	}
	if spec.MinDataDiskGi > 0 {
		out["minDataDiskGi"] = spec.MinDataDiskGi
	}
	if spec.NFS != nil {
		nfs := map[string]interface{}{
			"enabled": spec.NFSEnabled(),
		}
		if spec.NFS.Server != "" {
			nfs["server"] = spec.NFS.Server
		}
		if spec.NFS.ExportBase != "" {
			nfs["exportBase"] = spec.NFS.ExportBase
		}
		if spec.NFS.ClusterPath != "" {
			nfs["clusterPath"] = spec.NFS.ClusterPath
		}
		if spec.NFS.StorageClass != "" {
			nfs["storageClass"] = spec.NFS.StorageClass
		}
		if spec.NFS.DelegateHost != "" {
			nfs["delegateHost"] = spec.NFS.DelegateHost
		}
		if spec.NFS.MountPath != "" {
			nfs["mountPath"] = spec.NFS.MountPath
		}
		if len(nfs) > 0 {
			out["nfs"] = nfs
		}
	}
	if spec.Longhorn != nil {
		lh := map[string]interface{}{
			"enabled": spec.LonghornEnabled(),
		}
		if dev := spec.EffectiveLonghornDevice(); dev != "" {
			lh["device"] = dev
		}
		if sc := spec.EffectiveLonghornStorageClass(); sc != "" {
			lh["storageClass"] = sc
		}
		out["longhorn"] = lh
	}
	return out
}
