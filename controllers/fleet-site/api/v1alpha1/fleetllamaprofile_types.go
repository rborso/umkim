package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	DefaultLlamaCppChart        = "llama-cpp-helm"
	DefaultLlamaCppChartVersion = "0.1.4"

	AnnotationLlamaProfileTest = "fleet.umkim.com/llama-profile-test"
	AnnotationLlamaProfileName = "fleet.umkim.com/llama-profile"

	LlamaProfileTestPhasePending     = "Pending"
	LlamaProfileTestPhaseInstalling  = "Installing"
	LlamaProfileTestPhaseVerifying   = "Verifying"
	LlamaProfileTestPhaseReady       = "Ready"
	LlamaProfileTestPhaseSucceeded   = "Succeeded"
	LlamaProfileTestPhaseFailed      = "Failed"
	LlamaProfileTestPhaseCleaningUp  = "CleaningUp"

	LlamaProfileTestModeQuick    = "quick"
	LlamaProfileTestModeExtended = "extended"
)

type FleetLlamaProfileSpec struct {
	Description  string `json:"description,omitempty"`
	ProfileYAML  string `json:"profileYaml,omitempty"`
	Chart        string `json:"chart,omitempty"`
	ChartVersion string `json:"chartVersion,omitempty"`
	System       bool   `json:"system,omitempty"`
}

type FleetLlamaProfileStatus struct {
	MergedValuesHash string `json:"mergedValuesHash,omitempty"`
}

type FleetLlamaProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetLlamaProfileSpec   `json:"spec,omitempty"`
	Status            FleetLlamaProfileStatus `json:"status,omitempty"`
}

type FleetLlamaProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetLlamaProfile `json:"items"`
}

func (p *FleetLlamaProfile) EffectiveChart() string {
	if p.Spec.Chart != "" {
		return p.Spec.Chart
	}
	return DefaultLlamaCppChart
}

func (p *FleetLlamaProfile) EffectiveChartVersion() string {
	if p.Spec.ChartVersion != "" {
		return p.Spec.ChartVersion
	}
	return DefaultLlamaCppChartVersion
}

type FleetLlamaProfileTestSpec struct {
	ProfileName  string `json:"profileName"`
	ClusterName  string `json:"clusterName"`
	Mode         string `json:"mode"`
	FleetAppName string `json:"fleetAppName,omitempty"`
	ChartVersion string `json:"chartVersion,omitempty"`
}

type FleetLlamaProfileTestStatus struct {
	Phase    string `json:"phase,omitempty"`
	APIBase  string `json:"apiBase,omitempty"`
	Model    string `json:"model,omitempty"`
	Message  string `json:"message,omitempty"`
}

type FleetLlamaProfileTest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetLlamaProfileTestSpec   `json:"spec,omitempty"`
	Status            FleetLlamaProfileTestStatus `json:"status,omitempty"`
}

type FleetLlamaProfileTestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetLlamaProfileTest `json:"items"`
}
