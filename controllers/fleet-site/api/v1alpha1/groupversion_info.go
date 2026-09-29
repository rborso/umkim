package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

const (
	Group   = "fleet.umkim.com"
	Version = "v1alpha1"
)

var (
	GroupVersion  = schema.GroupVersion{Group: Group, Version: Version}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

type FleetSite struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetSiteSpec   `json:"spec,omitempty"`
	Status            FleetSiteStatus `json:"status,omitempty"`
}

type FleetSiteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetSite `json:"items"`
}

type FleetClusterJoin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetClusterJoinSpec   `json:"spec,omitempty"`
	Status            FleetClusterJoinStatus `json:"status,omitempty"`
}

type FleetClusterJoinList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetClusterJoin `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FleetSite{}, &FleetSiteList{})
	SchemeBuilder.Register(&FleetClusterJoin{}, &FleetClusterJoinList{})
	SchemeBuilder.Register(&FleetIngressAuth{}, &FleetIngressAuthList{})
	SchemeBuilder.Register(&FleetAcme{}, &FleetAcmeList{})
	SchemeBuilder.Register(&FleetLLM{}, &FleetLLMList{})
	SchemeBuilder.Register(&FleetInstanceBackup{}, &FleetInstanceBackupList{})
	SchemeBuilder.Register(&FleetInstanceRestore{}, &FleetInstanceRestoreList{})
	SchemeBuilder.Register(&FleetApp{}, &FleetAppList{})
	SchemeBuilder.Register(&FleetChart{}, &FleetChartList{})
	SchemeBuilder.Register(&FleetLlamaProfile{}, &FleetLlamaProfileList{})
	SchemeBuilder.Register(&FleetLlamaProfileTest{}, &FleetLlamaProfileTestList{})
}
