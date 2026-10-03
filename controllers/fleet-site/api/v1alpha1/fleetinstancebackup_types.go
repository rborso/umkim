package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	BackupPhasePending  = "Pending"
	BackupPhaseRunning  = "Running"
	BackupPhaseComplete = "Complete"
	BackupPhaseFailed   = "Failed"
)

type FleetInstanceBackupSpec struct {
	InstanceID string `json:"instanceId"`
	Cluster    string `json:"cluster"`
	Release    string `json:"release"`
	Namespace  string `json:"namespace,omitempty"`
	Wait       bool   `json:"wait,omitempty"`
}

type FleetInstanceBackupStatus struct {
	Phase           string       `json:"phase,omitempty"`
	Message         string       `json:"message,omitempty"`
	VeleroBackupName string      `json:"veleroBackupName,omitempty"`
	JobName         string       `json:"jobName,omitempty"`
	ArtifactSecret  string       `json:"artifactSecret,omitempty"`
	StartedAt       *metav1.Time `json:"startedAt,omitempty"`
	CompletedAt     *metav1.Time `json:"completedAt,omitempty"`
	ObservedGeneration int64     `json:"observedGeneration,omitempty"`
}

type FleetInstanceBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetInstanceBackupSpec   `json:"spec,omitempty"`
	Status            FleetInstanceBackupStatus `json:"status,omitempty"`
}

type FleetInstanceBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetInstanceBackup `json:"items"`
}
