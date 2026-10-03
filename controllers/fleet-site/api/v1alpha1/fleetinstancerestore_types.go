package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	RestorePhasePending   = "Pending"
	RestorePhaseRestoring = "Restoring"
	RestorePhaseRebinding = "Rebinding"
	RestorePhaseComplete  = "Complete"
	RestorePhaseFailed    = "Failed"

	RebindModeSupersede = "supersede"
	RebindModeClone     = "clone"
)

type FleetInstanceRestoreRebindSpec struct {
	Enabled           bool              `json:"enabled,omitempty"`
	Mode              string            `json:"mode,omitempty"`
	DisplayNameSuffix string            `json:"displayNameSuffix,omitempty"`
	UserValuePatches  []UserValuePatch  `json:"userValuePatches,omitempty"`
}

type UserValuePatch struct {
	Path  string `json:"path"`
	Value string `json:"value"`
}

type FleetInstanceRestoreSpec struct {
	BackupRef          string                         `json:"backupRef"`
	BackupCRName       string                         `json:"backupCRName,omitempty"`
	VeleroBackupName   string                         `json:"veleroBackupName,omitempty"`
	InstanceID         string                         `json:"instanceId,omitempty"`
	Release            string                         `json:"release,omitempty"`
	Namespace          string                         `json:"namespace,omitempty"`
	TargetCluster      string                         `json:"targetCluster"`
	SourceCluster      string                         `json:"sourceCluster,omitempty"`
	Rebind             *FleetInstanceRestoreRebindSpec `json:"rebind,omitempty"`
	StorageClassHints  map[string]string              `json:"storageClassHints,omitempty"`
}

type FleetInstanceRestoreStatus struct {
	Phase            string       `json:"phase,omitempty"`
	Message          string       `json:"message,omitempty"`
	VeleroRestoreName string      `json:"veleroRestoreName,omitempty"`
	JobName          string       `json:"jobName,omitempty"`
	NewInstanceID    string       `json:"newInstanceId,omitempty"`
	StartedAt        *metav1.Time `json:"startedAt,omitempty"`
	CompletedAt      *metav1.Time `json:"completedAt,omitempty"`
	ObservedGeneration int64     `json:"observedGeneration,omitempty"`
}

type FleetInstanceRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetInstanceRestoreSpec   `json:"spec,omitempty"`
	Status            FleetInstanceRestoreStatus `json:"status,omitempty"`
}

type FleetInstanceRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetInstanceRestore `json:"items"`
}
