package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/backup"
	"github.com/rborso/umkim/controllers/fleet-site/internal/dcm"
	"github.com/rborso/umkim/controllers/fleet-site/internal/workload"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *Server) dcmClient() *dcm.Client {
	return dcm.NewClient(s.Cfg.DCMCatalogAPIURL)
}

func (s *Server) handleBackupInfrastructure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := backup.CheckInfrastructure(r.Context(), s.Client, s.Cfg.Namespace, "fleet-system")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleInstanceWorkloads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	instanceID := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/instance-workloads/")
	instanceID = strings.Trim(instanceID, "/")
	if instanceID == "" {
		writeAPIError(w, http.StatusBadRequest, "instance id required", "INVALID_REQUEST")
		return
	}
	resolved, err := workload.Resolve(r.Context(), s.dcmClient(), instanceID)
	if err != nil {
		if strings.Contains(err.Error(), "not deployed") {
			writeAPIError(w, http.StatusConflict, err.Error(), "WORKLOAD_NOT_DEPLOYED")
			return
		}
		if strings.Contains(err.Error(), "not found") {
			writeAPIError(w, http.StatusNotFound, err.Error(), "INSTANCE_NOT_FOUND")
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.enrichWorkloadRoute(r.Context(), resolved)
	writeJSON(w, http.StatusOK, resolved)
}

func (s *Server) enrichWorkloadRoute(ctx context.Context, resolved *workload.Resolved) {
	fleetedgeList, err := s.listAppRoutes(ctx)
	if err != nil {
		return
	}
	for _, route := range fleetedgeList {
		if route.instanceID != resolved.InstanceID {
			continue
		}
		resolved.HubHost = route.hubHost
		resolved.ActiveCluster = route.activeCluster
		if resolved.ActiveCluster != "" {
			resolved.Cluster = resolved.ActiveCluster
		}
		break
	}
}

type appRouteSummary struct {
	instanceID    string
	hubHost       string
	activeCluster string
}

func (s *Server) listAppRoutes(ctx context.Context) ([]appRouteSummary, error) {
	// Reuse unstructured list from instance_route - import fleetedge
	return listFleetAppRoutes(ctx, s.Client, s.Cfg.EdgeRoutesNamespace)
}

func (s *Server) handleInstanceBackups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listInstanceBackups(r.Context(), w, r)
	case http.MethodPost:
		s.createInstanceBackup(r.Context(), w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleInstanceBackupByName(w http.ResponseWriter, r *http.Request, name string) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		var fib fleetv1alpha1.FleetInstanceBackup
		if err := s.Client.Get(ctx, client.ObjectKey{Namespace: s.Cfg.Namespace, Name: name}, &fib); err != nil {
			if apierrors.IsNotFound(err) {
				writeAPIError(w, http.StatusNotFound, "backup not found", "NOT_FOUND")
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, backupToResponse(&fib))
	case http.MethodDelete:
		var fib fleetv1alpha1.FleetInstanceBackup
		if err := s.Client.Get(ctx, client.ObjectKey{Namespace: s.Cfg.Namespace, Name: name}, &fib); err != nil {
			if apierrors.IsNotFound(err) {
				writeAPIError(w, http.StatusNotFound, "backup not found", "NOT_FOUND")
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.Client.Delete(ctx, &fib); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type createBackupRequest struct {
	InstanceID string `json:"instance_id"`
	Cluster    string `json:"cluster"`
	Release    string `json:"release"`
	Namespace  string `json:"namespace"`
	Wait       *bool  `json:"wait"`
}

func (s *Server) createInstanceBackup(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req createBackupRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.InstanceID) == "" {
		writeAPIError(w, http.StatusBadRequest, "instance_id required", "INVALID_REQUEST")
		return
	}
	resolved, err := workload.Resolve(ctx, s.dcmClient(), req.InstanceID)
	if err != nil && req.Cluster == "" {
		if strings.Contains(err.Error(), "not found") {
			writeAPIError(w, http.StatusNotFound, err.Error(), "INSTANCE_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusConflict, err.Error(), "WORKLOAD_NOT_DEPLOYED")
		return
	}
	cluster := req.Cluster
	release := req.Release
	namespace := req.Namespace
	if resolved != nil {
		if cluster == "" {
			cluster = resolved.Cluster
		}
		if release == "" {
			release = resolved.Release
		}
		if namespace == "" {
			namespace = resolved.Namespace
		}
	}
	if cluster == "" || release == "" {
		writeAPIError(w, http.StatusBadRequest, "cluster and release required", "INVALID_REQUEST")
		return
	}
	infra, err := backup.CheckInfrastructure(ctx, s.Client, s.Cfg.Namespace, "fleet-system")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := backup.ClusterInfraOK(infra, cluster); err != nil {
		writeAPIError(w, http.StatusConflict, err.Error(), "BACKUP_INFRA_NOT_READY")
		return
	}
	if busy, _ := s.instanceOperationBusy(ctx, req.InstanceID); busy {
		writeAPIError(w, http.StatusConflict, "backup or restore already running for instance", "CONCURRENT_OPERATION")
		return
	}
	wait := true
	if req.Wait != nil {
		wait = *req.Wait
	}
	stamp := time.Now().UTC().Format("20060102150405")
	crName := fmt.Sprintf("backup-%s-%s", sanitizeBackupToken(release), stamp)
	if len(crName) > 63 {
		crName = crName[:63]
	}
	crName = strings.TrimRight(crName, "-")
	fib := &fleetv1alpha1.FleetInstanceBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: s.Cfg.Namespace,
			Labels: map[string]string{
				"fleet.umkim.com/instance-id": req.InstanceID,
			},
		},
		Spec: fleetv1alpha1.FleetInstanceBackupSpec{
			InstanceID: req.InstanceID,
			Cluster:    cluster,
			Release:    release,
			Namespace:  namespace,
			Wait:       wait,
		},
		Status: fleetv1alpha1.FleetInstanceBackupStatus{
			Phase: fleetv1alpha1.BackupPhasePending,
		},
	}
	if err := s.Client.Create(ctx, fib); err != nil {
		if apierrors.IsAlreadyExists(err) {
			writeAPIError(w, http.StatusConflict, "backup name exists", "ALREADY_EXISTS")
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"name":  fib.Name,
		"phase": fib.Status.Phase,
	})
}

func (s *Server) listInstanceBackups(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var list fleetv1alpha1.FleetInstanceBackupList
	opts := []client.ListOption{client.InNamespace(s.Cfg.Namespace)}
	if id := r.URL.Query().Get("instance_id"); id != "" {
		opts = append(opts, client.MatchingLabels{"fleet.umkim.com/instance-id": id})
	}
	if err := s.Client.List(ctx, &list, opts...); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	phaseFilter := r.URL.Query().Get("phase")
	clusterFilter := r.URL.Query().Get("cluster")
	items := make([]backupResponse, 0, len(list.Items))
	for i := range list.Items {
		fib := &list.Items[i]
		if phaseFilter != "" && fib.Status.Phase != phaseFilter {
			continue
		}
		if clusterFilter != "" && fib.Spec.Cluster != clusterFilter {
			continue
		}
		items = append(items, backupToResponse(fib))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"backups": items})
}

type backupResponse struct {
	Name             string `json:"name"`
	InstanceID       string `json:"instance_id"`
	Cluster          string `json:"cluster"`
	Release          string `json:"release"`
	Namespace        string `json:"namespace,omitempty"`
	Phase            string `json:"phase"`
	Message          string `json:"message,omitempty"`
	VeleroBackupName string `json:"velero_backup_name,omitempty"`
	ArtifactSecret   string `json:"artifact_secret,omitempty"`
	StartedAt        string `json:"started_at,omitempty"`
	CompletedAt      string `json:"completed_at,omitempty"`
}

func backupToResponse(fib *fleetv1alpha1.FleetInstanceBackup) backupResponse {
	resp := backupResponse{
		Name:             fib.Name,
		InstanceID:       fib.Spec.InstanceID,
		Cluster:          fib.Spec.Cluster,
		Release:          fib.Spec.Release,
		Namespace:        fib.Spec.Namespace,
		Phase:            fib.Status.Phase,
		Message:          fib.Status.Message,
		VeleroBackupName: fib.Status.VeleroBackupName,
		ArtifactSecret:   fib.Status.ArtifactSecret,
	}
	if fib.Status.StartedAt != nil {
		resp.StartedAt = fib.Status.StartedAt.Format(time.RFC3339)
	}
	if fib.Status.CompletedAt != nil {
		resp.CompletedAt = fib.Status.CompletedAt.Format(time.RFC3339)
	}
	return resp
}

func sanitizeBackupToken(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "inst"
	}
	return out
}

func (s *Server) handleInstanceRestores(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listInstanceRestores(r.Context(), w, r)
	case http.MethodPost:
		s.createInstanceRestore(r.Context(), w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleInstanceRestoreByName(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var fir fleetv1alpha1.FleetInstanceRestore
	if err := s.Client.Get(r.Context(), client.ObjectKey{Namespace: s.Cfg.Namespace, Name: name}, &fir); err != nil {
		if apierrors.IsNotFound(err) {
			writeAPIError(w, http.StatusNotFound, "restore not found", "NOT_FOUND")
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, restoreToResponse(&fir))
}

type restoreRebindRequest struct {
	Enabled           bool                         `json:"enabled"`
	Mode              string                       `json:"mode"`
	DisplayNameSuffix string                       `json:"display_name_suffix"`
	UserValuePatches  []fleetv1alpha1.UserValuePatch `json:"user_value_patches"`
}

type createRestoreRequest struct {
	BackupName        string              `json:"backup_name"`
	TargetCluster     string              `json:"target_cluster"`
	SourceCluster     string              `json:"source_cluster"`
	Rebind            *restoreRebindRequest `json:"rebind"`
	StorageClassHints map[string]string   `json:"storage_class_hints"`
}

func (s *Server) createInstanceRestore(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req createRestoreRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.BackupName == "" || req.TargetCluster == "" {
		writeAPIError(w, http.StatusBadRequest, "backup_name and target_cluster required", "INVALID_REQUEST")
		return
	}
	infra, err := backup.CheckInfrastructure(ctx, s.Client, s.Cfg.Namespace, "fleet-system")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := backup.ClusterInfraOK(infra, req.TargetCluster); err != nil {
		writeAPIError(w, http.StatusConflict, err.Error(), "BACKUP_INFRA_NOT_READY")
		return
	}
	var fib fleetv1alpha1.FleetInstanceBackup
	backupCRName := ""
	instanceID := ""
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: s.Cfg.Namespace, Name: req.BackupName}, &fib); err == nil {
		backupCRName = fib.Name
		instanceID = fib.Spec.InstanceID
	} else {
		var list fleetv1alpha1.FleetInstanceBackupList
		if err := s.Client.List(ctx, &list, client.InNamespace(s.Cfg.Namespace)); err == nil {
			for i := range list.Items {
				if list.Items[i].Status.VeleroBackupName == req.BackupName {
					fib = list.Items[i]
					backupCRName = fib.Name
					instanceID = fib.Spec.InstanceID
					break
				}
			}
		}
	}
	if instanceID != "" {
		if busy, _ := s.instanceOperationBusy(ctx, instanceID); busy {
			writeAPIError(w, http.StatusConflict, "backup or restore already running for instance", "CONCURRENT_OPERATION")
			return
		}
	}
	stamp := time.Now().UTC().Format("20060102150405")
	crName := fmt.Sprintf("restore-%s-%s", sanitizeBackupToken(req.TargetCluster), stamp)
	if len(crName) > 63 {
		crName = crName[:63]
	}
	spec := fleetv1alpha1.FleetInstanceRestoreSpec{
		BackupRef:        req.BackupName,
		BackupCRName:     backupCRName,
		TargetCluster:    req.TargetCluster,
		SourceCluster:    req.SourceCluster,
		InstanceID:       instanceID,
		StorageClassHints: req.StorageClassHints,
	}
	if backupCRName != "" {
		spec.VeleroBackupName = fib.Status.VeleroBackupName
		spec.Release = fib.Spec.Release
		spec.Namespace = fib.Spec.Namespace
	} else {
		spec.VeleroBackupName = req.BackupName
	}
	if req.Rebind != nil {
		mode := req.Rebind.Mode
		if mode == "" && req.Rebind.Enabled {
			mode = fleetv1alpha1.RebindModeSupersede
		}
		spec.Rebind = &fleetv1alpha1.FleetInstanceRestoreRebindSpec{
			Enabled:           req.Rebind.Enabled,
			Mode:              mode,
			DisplayNameSuffix: req.Rebind.DisplayNameSuffix,
			UserValuePatches:  req.Rebind.UserValuePatches,
		}
	}
	fir := &fleetv1alpha1.FleetInstanceRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: s.Cfg.Namespace,
		},
		Spec: spec,
		Status: fleetv1alpha1.FleetInstanceRestoreStatus{
			Phase: fleetv1alpha1.RestorePhasePending,
		},
	}
	if instanceID != "" {
		fir.Labels = map[string]string{"fleet.umkim.com/instance-id": instanceID}
	}
	if err := s.Client.Create(ctx, fir); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"name":  fir.Name,
		"phase": fir.Status.Phase,
	})
}

func (s *Server) listInstanceRestores(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var list fleetv1alpha1.FleetInstanceRestoreList
	opts := []client.ListOption{client.InNamespace(s.Cfg.Namespace)}
	if id := r.URL.Query().Get("instance_id"); id != "" {
		opts = append(opts, client.MatchingLabels{"fleet.umkim.com/instance-id": id})
	}
	if err := s.Client.List(ctx, &list, opts...); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	backupFilter := r.URL.Query().Get("backup_name")
	items := make([]restoreResponse, 0, len(list.Items))
	for i := range list.Items {
		fir := &list.Items[i]
		if backupFilter != "" && fir.Spec.BackupRef != backupFilter && fir.Spec.VeleroBackupName != backupFilter {
			continue
		}
		items = append(items, restoreToResponse(fir))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"restores": items})
}

type restoreResponse struct {
	Name              string `json:"name"`
	BackupRef         string `json:"backup_ref"`
	TargetCluster     string `json:"target_cluster"`
	SourceCluster     string `json:"source_cluster,omitempty"`
	Phase             string `json:"phase"`
	Message           string `json:"message,omitempty"`
	VeleroRestoreName string `json:"velero_restore_name,omitempty"`
	NewInstanceID     string `json:"new_instance_id,omitempty"`
	StartedAt         string `json:"started_at,omitempty"`
	CompletedAt       string `json:"completed_at,omitempty"`
}

func restoreToResponse(fir *fleetv1alpha1.FleetInstanceRestore) restoreResponse {
	resp := restoreResponse{
		Name:              fir.Name,
		BackupRef:         fir.Spec.BackupRef,
		TargetCluster:     fir.Spec.TargetCluster,
		SourceCluster:     fir.Spec.SourceCluster,
		Phase:             fir.Status.Phase,
		Message:           fir.Status.Message,
		VeleroRestoreName: fir.Status.VeleroRestoreName,
		NewInstanceID:     fir.Status.NewInstanceID,
	}
	if fir.Status.StartedAt != nil {
		resp.StartedAt = fir.Status.StartedAt.Format(time.RFC3339)
	}
	if fir.Status.CompletedAt != nil {
		resp.CompletedAt = fir.Status.CompletedAt.Format(time.RFC3339)
	}
	return resp
}

func (s *Server) instanceOperationBusy(ctx context.Context, instanceID string) (bool, error) {
	var backups fleetv1alpha1.FleetInstanceBackupList
	if err := s.Client.List(ctx, &backups, client.InNamespace(s.Cfg.Namespace),
		client.MatchingLabels{"fleet.umkim.com/instance-id": instanceID}); err != nil {
		return false, err
	}
	for _, fib := range backups.Items {
		for _, phase := range backup.ActiveBackupPhases() {
			if fib.Status.Phase == phase {
				return true, nil
			}
		}
	}
	var restores fleetv1alpha1.FleetInstanceRestoreList
	if err := s.Client.List(ctx, &restores, client.InNamespace(s.Cfg.Namespace),
		client.MatchingLabels{"fleet.umkim.com/instance-id": instanceID}); err != nil {
		return false, err
	}
	for _, fir := range restores.Items {
		for _, phase := range backup.ActiveRestorePhases() {
			if fir.Status.Phase == phase {
				return true, nil
			}
		}
	}
	return false, nil
}

func writeAPIError(w http.ResponseWriter, code int, msg, errCode string) {
	writeJSON(w, code, map[string]string{"error": msg, "code": errCode})
}
