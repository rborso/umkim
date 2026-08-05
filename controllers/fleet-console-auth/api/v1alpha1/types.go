package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=frole
type FleetRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetRoleSpec `json:"spec,omitempty"`
}

type FleetRoleSpec struct {
	Rules []PolicyRule `json:"rules"`
}

type PolicyRule struct {
	Resources   []string `json:"resources"`
	Verbs       []string `json:"verbs"`
	AccessLevel string   `json:"accessLevel,omitempty"`
}

// +kubebuilder:object:root=true
type FleetRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetRole `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=frb
type FleetRoleBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetRoleBindingSpec `json:"spec,omitempty"`
}

type FleetRoleBindingSpec struct {
	Subjects []SubjectRef `json:"subjects"`
	RoleRef  RoleRef      `json:"roleRef"`
	Scopes   []ScopeRef   `json:"scopes,omitempty"`
}

type SubjectRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type RoleRef struct {
	Name string `json:"name"`
}

type ScopeRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// +kubebuilder:object:root=true
type FleetRoleBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetRoleBinding `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=fuser
type FleetUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetUserSpec   `json:"spec,omitempty"`
	Status            FleetUserStatus `json:"status,omitempty"`
}

type FleetUserSpec struct {
	ExternalID  string   `json:"externalId"`
	DisplayName string   `json:"displayName,omitempty"`
	Groups      []string `json:"groups,omitempty"`
	Disabled    bool     `json:"disabled,omitempty"`
	AuthMethod  string   `json:"authMethod,omitempty"`
}

type FleetUserStatus struct {
	LastLogin *metav1.Time `json:"lastLogin,omitempty"`
}

// +kubebuilder:object:root=true
type FleetUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetUser `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=fgroup
type FleetGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetGroupSpec `json:"spec,omitempty"`
}

type FleetGroupSpec struct {
	DisplayName  string `json:"displayName,omitempty"`
	LDAPGroupCN  string `json:"ldapGroupCN,omitempty"`
	Description  string `json:"description,omitempty"`
}

// +kubebuilder:object:root=true
type FleetGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetGroup `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=fsa
type FleetServiceAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetServiceAccountSpec   `json:"spec,omitempty"`
	Status            FleetServiceAccountStatus `json:"status,omitempty"`
}

type FleetServiceAccountSpec struct {
	DisplayName string       `json:"displayName,omitempty"`
	RoleBindings []RoleBindingInline `json:"roleBindings,omitempty"`
}

type RoleBindingInline struct {
	RoleRef RoleRef    `json:"roleRef"`
	Scopes  []ScopeRef `json:"scopes,omitempty"`
}

type FleetServiceAccountStatus struct {
	TokenSecretName string `json:"tokenSecretName,omitempty"`
}

// +kubebuilder:object:root=true
type FleetServiceAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetServiceAccount `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=fca
type FleetCatalogAccess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetCatalogAccessSpec `json:"spec,omitempty"`
}

type FleetCatalogAccessSpec struct {
	CatalogItemID string              `json:"catalogItemId"`
	Visibility    string              `json:"visibility"`
	Grants        []CatalogAccessGrant `json:"grants,omitempty"`
}

type CatalogAccessGrant struct {
	Subject     SubjectRef `json:"subject"`
	AccessLevel string     `json:"accessLevel"`
}

// +kubebuilder:object:root=true
type FleetCatalogAccessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetCatalogAccess `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=faudit
type FleetAuditEvent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetAuditEventSpec `json:"spec,omitempty"`
}

type FleetAuditEventSpec struct {
	Timestamp metav1.Time    `json:"timestamp"`
	Actor     SubjectRef     `json:"actor"`
	Action    string         `json:"action"`
	Resource  *ScopeRef      `json:"resource,omitempty"`
	Scope     *ScopeRef      `json:"scope,omitempty"`
	Result    string         `json:"result"`
	HTTP      *AuditHTTPInfo `json:"http,omitempty"`
	Message   string         `json:"message,omitempty"`
}

type AuditHTTPInfo struct {
	Method   string `json:"method"`
	Path     string `json:"path"`
	ClientIP string `json:"clientIP,omitempty"`
}

// +kubebuilder:object:root=true
type FleetAuditEventList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetAuditEvent `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=forg
type FleetOrg struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetOrgSpec   `json:"spec,omitempty"`
	Status            FleetOrgStatus `json:"status,omitempty"`
}

type FleetOrgSpec struct {
	// Slug is the immutable org identifier used in name prefixes and fleet.org labels.
	Slug        string           `json:"slug"`
	DisplayName string           `json:"displayName,omitempty"`
	SharedQuota *OrgResourceQuota `json:"sharedQuota,omitempty"`
}

type OrgResourceQuota struct {
	CPUMilli      int64 `json:"cpuMilli,omitempty"`
	MemoryBytes   int64 `json:"memoryBytes,omitempty"`
	GPUCount      int64 `json:"gpuCount,omitempty"`
	LlmTokensPerMonth int64 `json:"llmTokensPerMonth,omitempty"`
}

type FleetOrgStatus struct {
	SharedUsage *OrgResourceUsage `json:"sharedUsage,omitempty"`
}

type OrgResourceUsage struct {
	CPUMilli          int64 `json:"cpuMilli,omitempty"`
	MemoryBytes       int64 `json:"memoryBytes,omitempty"`
	GPUCount          int64 `json:"gpuCount,omitempty"`
	LlmTokensPerMonth int64 `json:"llmTokensPerMonth,omitempty"`
}

// +kubebuilder:object:root=true
type FleetOrgList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetOrg `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=flmak
type FleetLLMApiKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetLLMApiKeySpec   `json:"spec,omitempty"`
	Status            FleetLLMApiKeyStatus `json:"status,omitempty"`
}

type FleetLLMApiKeySpec struct {
	Org         string   `json:"org"`
	DisplayName string   `json:"displayName,omitempty"`
	SecretRef   string   `json:"secretRef,omitempty"`
	Models      []string `json:"models,omitempty"`
}

type FleetLLMApiKeyStatus struct {
	KeyPrefix string `json:"keyPrefix,omitempty"`
	Active    bool   `json:"active,omitempty"`
}

// +kubebuilder:object:root=true
type FleetLLMApiKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetLLMApiKey `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&FleetRole{}, &FleetRoleList{},
		&FleetRoleBinding{}, &FleetRoleBindingList{},
		&FleetUser{}, &FleetUserList{},
		&FleetGroup{}, &FleetGroupList{},
		&FleetServiceAccount{}, &FleetServiceAccountList{},
		&FleetCatalogAccess{}, &FleetCatalogAccessList{},
		&FleetAuditEvent{}, &FleetAuditEventList{},
		&FleetOrg{}, &FleetOrgList{},
		&FleetLLMApiKey{}, &FleetLLMApiKeyList{},
	)
}
