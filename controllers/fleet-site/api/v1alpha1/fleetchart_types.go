package v1alpha1

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ChartPhasePending    = "Pending"
	ChartPhaseFetching   = "Fetching"
	ChartPhasePublishing = "Publishing"
	ChartPhaseReady      = "Ready"
	ChartPhaseFailed     = "Failed"

	ChartSourceOCI    = "oci"
	ChartSourceHTTP   = "http"
	ChartSourceUpload = "upload"
	ChartSourceHarbor = "harbor"

	ChartImageExternal = "external"
	ChartImageProxy    = "proxy"
	ChartImageCopy     = "copy"
)

// FleetChart is a hub library entry for an OCI Helm chart in Harbor.
// Publish is separate from FleetApp install; there is no placement engine.
type FleetChartSpec struct {
	// Source is oci, http, upload, or harbor.
	Source string `json:"source"`
	// Origin is the upstream oci:// or http(s) URL. Empty for harbor-imported charts.
	Origin string `json:"origin,omitempty"`
	// Version is the chart version to publish. Empty uses the packaged Chart.yaml version.
	Version string `json:"version,omitempty"`
	// ImagePolicy is external, proxy, or copy. Default external.
	ImagePolicy string `json:"imagePolicy,omitempty"`
	// InboxFile is a basename under the hub chart inbox (upload source only).
	InboxFile string `json:"inboxFile,omitempty"`
	// ValuesYAML is optional Helm values used when rendering for image copy.
	ValuesYAML string `json:"valuesYaml,omitempty"`
}

type FleetChartCopiedImage struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type FleetChartStatus struct {
	Phase              string                  `json:"phase,omitempty"`
	Message            string                  `json:"message,omitempty"`
	LastJobName        string                  `json:"lastJobName,omitempty"`
	ObservedGeneration int64                   `json:"observedGeneration,omitempty"`
	SpecHash           string                  `json:"specHash,omitempty"`
	Chart              string                  `json:"chart,omitempty"`
	Digest             string                  `json:"digest,omitempty"`
	Versions           []string                `json:"versions,omitempty"`
	ImageValues        string                  `json:"imageValues,omitempty"`
	ImagesCopied       []FleetChartCopiedImage `json:"imagesCopied,omitempty"`
	// OIDCDefaults is the app's recommended Authelia OIDC client (JSON), read
	// from the chart's `umkim.oidc` values at publish time. Used to pre-fill the
	// install prompt.
	OIDCDefaults string `json:"oidcDefaults,omitempty"`
	// TenantDefault is the app's recommended route tenant, read from the chart's
	// `umkim.tenant` values at publish time. Every directory is created with a
	// `default-tenant` group (see the LLDAP bootstrap), so this is the tenant that
	// exists everywhere without an operator declaring one. Used to pre-fill the
	// install prompt's requireAuthGroups.
	TenantDefault string `json:"tenantDefault,omitempty"`
	Conditions         []metav1.Condition      `json:"conditions,omitempty"`
}

type FleetChart struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetChartSpec   `json:"spec,omitempty"`
	Status            FleetChartStatus `json:"status,omitempty"`
}

type FleetChartList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetChart `json:"items"`
}

func (s FleetChartSpec) EffectiveImagePolicy() string {
	p := strings.ToLower(strings.TrimSpace(s.ImagePolicy))
	if p == "" {
		return ChartImageExternal
	}
	return p
}

func (s FleetChartSpec) EffectiveSource() string {
	return strings.ToLower(strings.TrimSpace(s.Source))
}
