package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	FleetIngressAuthName = "fleet-ingress-auth"

	IngressAuthPhaseDisabled = "Disabled"
	IngressAuthPhaseApplying = "Applying"
	IngressAuthPhaseReady    = "Ready"
	IngressAuthPhaseFailed   = "Failed"

	IngressAuthClusterPhasePending  = "Pending"
	IngressAuthClusterPhaseApplying = "Applying"
	IngressAuthClusterPhaseReady    = "Ready"
	IngressAuthClusterPhaseFailed   = "Failed"
	IngressAuthClusterPhaseDisabled = "Disabled"

	LldapWebUIPhaseDisabled = "Disabled"
	LldapWebUIPhaseEnabling = "Enabling"
	LldapWebUIPhaseReady    = "Ready"
	LldapWebUIPhaseFailed   = "Failed"

	DefaultIngressAuthSecretsName = "fleet-ingress-auth-secrets"
)

type FleetIngressAuthSpec struct {
	HubEnabled          bool     `json:"hubEnabled,omitempty"`
	HubExposeLldapWebUI bool     `json:"hubExposeLldapWebUI,omitempty"`
	PortalHost          string   `json:"portalHost,omitempty"`
	CookieDomain string   `json:"cookieDomain,omitempty"`
	AccessDomains []string `json:"accessDomains,omitempty"`
	SecretsRef   string   `json:"secretsRef,omitempty"`
}

func (s *FleetIngressAuthSpec) EffectiveSecretsRef() string {
	if s.SecretsRef != "" {
		return s.SecretsRef
	}
	return DefaultIngressAuthSecretsName
}

type FleetIngressAuthHubStatus struct {
	Phase              string `json:"phase,omitempty"`
	Message            string `json:"message,omitempty"`
	AutheliaServiceURL string `json:"autheliaServiceURL,omitempty"`
	LLDAPWebUIEnabled  bool   `json:"lldapWebUIEnabled,omitempty"`
	LLDAPWebUIURL      string `json:"lldapWebUIURL,omitempty"`
	LLDAPWebUIPhase    string `json:"lldapWebUIPhase,omitempty"`
	LLDAPWebUIMessage  string `json:"lldapWebUIMessage,omitempty"`
}

type FleetIngressAuthClusterStatus struct {
	Name    string `json:"name"`
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

type FleetIngressAuthStatus struct {
	Phase              string                          `json:"phase,omitempty"`
	Message            string                          `json:"message,omitempty"`
	Hub                FleetIngressAuthHubStatus       `json:"hub,omitempty"`
	Clusters           []FleetIngressAuthClusterStatus `json:"clusters,omitempty"`
	LastJobName        string                          `json:"lastJobName,omitempty"`
	ObservedGeneration int64                           `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition              `json:"conditions,omitempty"`
}

type FleetIngressAuth struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetIngressAuthSpec   `json:"spec,omitempty"`
	Status            FleetIngressAuthStatus `json:"status,omitempty"`
}

type FleetIngressAuthList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetIngressAuth `json:"items"`
}

type FleetClusterJoinIngressAuthSpec struct {
	Enabled          bool `json:"enabled,omitempty"`
	ExposeLLDAPWebUI bool `json:"exposeLldapWebUI,omitempty"`
	// Directory selects where the LLDAP directory lives: "" / "local" runs its own
	// LLDAP on the spoke; "hub" uses the shared hub directory over WireGuard.
	Directory string `json:"directory,omitempty"`
	// OIDCClients registers Authelia OIDC clients for OIDC-only applications
	// (e.g. Immich). The client secret is generated and stored by UMKIM.
	OIDCClients []IngressAuthOIDCClient `json:"oidcClients,omitempty"`
}

// IngressAuthOIDCClient declares one Authelia OIDC client.
type IngressAuthOIDCClient struct {
	ClientID     string   `json:"clientId"`
	ClientName   string   `json:"clientName,omitempty"`
	RedirectURIs []string `json:"redirectUris,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	// RequirePKCE defaults to true; set false for clients that do not send a
	// code_challenge (e.g. django-allauth).
	RequirePKCE *bool `json:"requirePkce,omitempty"`
}

type FleetClusterJoinIngressAuthStatus struct {
	Phase          string   `json:"phase,omitempty"`
	Message        string   `json:"message,omitempty"`
	JobName        string   `json:"jobName,omitempty"`
	SecretsRef     string   `json:"secretsRef,omitempty"`
	PortalHost     string   `json:"portalHost,omitempty"`
	LLDAPWebUIHost string   `json:"lldapWebUIHost,omitempty"`
	AccessDomains  []string `json:"accessDomains,omitempty"`
}
