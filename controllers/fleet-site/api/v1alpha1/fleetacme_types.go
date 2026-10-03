package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	FleetAcmeName = "fleet-acme"

	AcmePhaseDisabled = "Disabled"
	AcmePhaseApplying = "Applying"
	AcmePhaseReady    = "Ready"
	AcmePhaseFailed   = "Failed"

	AcmeClusterPhasePending  = "Pending"
	AcmeClusterPhaseApplying = "Applying"
	AcmeClusterPhaseReady    = "Ready"
	AcmeClusterPhaseFailed   = "Failed"
	AcmeClusterPhaseDisabled = "Disabled"
)

type FleetAcmeSpec struct {
	HubEnabled bool   `json:"hubEnabled,omitempty"`
	Staging    bool   `json:"staging,omitempty"`
	Email      string `json:"email,omitempty"`
}

type FleetAcmeHubStatus struct {
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

type FleetAcmeClusterStatus struct {
	Name    string `json:"name"`
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

type FleetAcmeStatus struct {
	Phase              string                   `json:"phase,omitempty"`
	Message            string                   `json:"message,omitempty"`
	Hub                FleetAcmeHubStatus       `json:"hub,omitempty"`
	Clusters           []FleetAcmeClusterStatus `json:"clusters,omitempty"`
	LastJobName        string                   `json:"lastJobName,omitempty"`
	ObservedGeneration int64                    `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition       `json:"conditions,omitempty"`
}

type FleetAcme struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetAcmeSpec   `json:"spec,omitempty"`
	Status            FleetAcmeStatus `json:"status,omitempty"`
}

type FleetAcmeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetAcme `json:"items"`
}

type FleetClusterJoinAcmeSpec struct {
	Enabled bool `json:"enabled,omitempty"`
}

type FleetClusterJoinAcmeStatus struct {
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
	JobName string `json:"jobName,omitempty"`
}
