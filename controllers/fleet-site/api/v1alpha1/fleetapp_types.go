package v1alpha1

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	FleetAppFinalizer = "fleet.umkim.com/helm"

	// FleetAppAnnotationLLMBackend opts a llama-cpp FleetApp into hub LiteLLM backend discovery.
	// Values: "true", "false". Absent means include (legacy installs and CLI without the flag).
	FleetAppAnnotationLLMBackend = "fleet.umkim.com/llm-backend"

	AppPhasePending    = "Pending"
	AppPhaseInstalling = "Installing"
	AppPhaseReady      = "Ready"
	AppPhaseFailed     = "Failed"
	AppPhaseDeleting   = "Deleting"

	AppRouteModeNone = "none"
	AppRouteModeHub  = "hub"
	AppRouteModeSite = "site"
)

// FleetAppRouteIntent is the operator-selected public route for a Helm release.
// There is no placement engine: traffic is always the cluster in spec.clusterName.
type FleetAppRouteIntent struct {
	Mode             string `json:"mode,omitempty"`
	HubHost          string `json:"hubHost,omitempty"`
	SiteHost         string `json:"siteHost,omitempty"`
	RequireAuth      bool   `json:"requireAuth,omitempty"`
	ServiceName      string `json:"serviceName,omitempty"`
	ServicePort      int32  `json:"servicePort,omitempty"`
	ServiceNamespace string `json:"serviceNamespace,omitempty"`
	HealthPath       string `json:"healthPath,omitempty"`
}

type FleetAppSpec struct {
	// ClusterName is the Ready FleetClusterJoin / ManagedCluster to install on.
	// Required. The hub is not a valid target.
	ClusterName string `json:"clusterName"`
	// Chart is an OCI chart reference (oci://host/project/name) or a short
	// name resolved against the hub Harbor charts prefix.
	Chart string `json:"chart"`
	// Version is the chart version. Empty means the registry default.
	Version string `json:"version,omitempty"`
	// ReleaseName is the Helm release. Defaults to the FleetApp metadata name.
	ReleaseName string `json:"releaseName,omitempty"`
	// Namespace is the spoke namespace. Defaults to default.
	Namespace string `json:"namespace,omitempty"`
	// ValuesYAML is a Helm values document applied with -f.
	ValuesYAML string              `json:"valuesYaml,omitempty"`
	Route      FleetAppRouteIntent `json:"route,omitempty"`
}

type FleetAppStatus struct {
	Phase              string             `json:"phase,omitempty"`
	Message            string             `json:"message,omitempty"`
	LastJobName        string             `json:"lastJobName,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	SpecHash           string             `json:"specHash,omitempty"`
	ReleaseName        string             `json:"releaseName,omitempty"`
	Namespace          string             `json:"namespace,omitempty"`
	ClusterName        string             `json:"clusterName,omitempty"`
	Chart              string             `json:"chart,omitempty"`
	RouteName          string             `json:"routeName,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

type FleetApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetAppSpec   `json:"spec,omitempty"`
	Status            FleetAppStatus `json:"status,omitempty"`
}

type FleetAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetApp `json:"items"`
}

func (s FleetAppSpec) EffectiveReleaseName(fallback string) string {
	if n := strings.TrimSpace(s.ReleaseName); n != "" {
		return n
	}
	return strings.TrimSpace(fallback)
}

func (s FleetAppSpec) EffectiveNamespace() string {
	if n := strings.TrimSpace(s.Namespace); n != "" {
		return n
	}
	return "default"
}

func (s FleetAppSpec) EffectiveRouteMode() string {
	m := strings.ToLower(strings.TrimSpace(s.Route.Mode))
	if m == "" {
		return AppRouteModeNone
	}
	return m
}

func (s FleetAppSpec) EffectiveServiceName(fallback string) string {
	if n := strings.TrimSpace(s.Route.ServiceName); n != "" {
		return n
	}
	return s.EffectiveReleaseName(fallback)
}

func (s FleetAppSpec) EffectiveServiceNamespace() string {
	if n := strings.TrimSpace(s.Route.ServiceNamespace); n != "" {
		return n
	}
	return s.EffectiveNamespace()
}

func (s FleetAppSpec) EffectiveServicePort() int32 {
	if s.Route.ServicePort > 0 {
		return s.Route.ServicePort
	}
	return 80
}

func (s FleetAppSpec) EffectiveHealthPath() string {
	if p := strings.TrimSpace(s.Route.HealthPath); p != "" {
		return p
	}
	return "/"
}

// RegistersLLMBackend reports whether a Ready llama-cpp FleetApp should appear in FleetLLM auto-discovery.
func (app *FleetApp) RegistersLLMBackend() bool {
	if app == nil {
		return false
	}
	if app.Annotations == nil {
		return true
	}
	v := strings.TrimSpace(app.Annotations[FleetAppAnnotationLLMBackend])
	if v == "" {
		return true
	}
	return v == "true"
}

// SetLLMBackendAnnotation records opt-in/out for hub LiteLLM backend discovery.
func (app *FleetApp) SetLLMBackendAnnotation(enabled bool) {
	if app.Annotations == nil {
		app.Annotations = map[string]string{}
	}
	if enabled {
		app.Annotations[FleetAppAnnotationLLMBackend] = "true"
	} else {
		app.Annotations[FleetAppAnnotationLLMBackend] = "false"
	}
}
