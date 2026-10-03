package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	FleetLLMName = "fleet-llm"

	LLMPhaseDisabled = "Disabled"
	LLMPhaseApplying = "Applying"
	LLMPhaseReady    = "Ready"
	LLMPhaseFailed   = "Failed"
)

type FleetLLMBackend struct {
	Model   string `json:"model,omitempty"`
	APIBase string `json:"apiBase,omitempty"`
}

type FleetLLMSpec struct {
	HubEnabled         bool              `json:"hubEnabled,omitempty"`
	PublicHost         string            `json:"publicHost,omitempty"`
	MasterKeySecretRef string            `json:"masterKeySecretRef,omitempty"`
	Backends           []FleetLLMBackend `json:"backends,omitempty"`
}

type FleetLLMHubStatus struct {
	Phase    string `json:"phase,omitempty"`
	Message  string `json:"message,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

type FleetLLMStatus struct {
	Phase              string              `json:"phase,omitempty"`
	Message            string              `json:"message,omitempty"`
	Hub                FleetLLMHubStatus   `json:"hub,omitempty"`
	LastJobName        string              `json:"lastJobName,omitempty"`
	ObservedGeneration int64               `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition  `json:"conditions,omitempty"`
}

type FleetLLM struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetLLMSpec   `json:"spec,omitempty"`
	Status            FleetLLMStatus `json:"status,omitempty"`
}

type FleetLLMList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetLLM `json:"items"`
}

func (f *FleetLLMSpec) EffectiveMasterKeySecretRef() string {
	if f.MasterKeySecretRef != "" {
		return f.MasterKeySecretRef
	}
	return "fleet-llm-master-key"
}

func (f *FleetLLMSpec) EffectivePublicHost(domain string) string {
	if f.PublicHost != "" {
		return f.PublicHost
	}
	if domain != "" {
		return "llm.api." + domain
	}
	return "llm.api.umkim.com"
}
