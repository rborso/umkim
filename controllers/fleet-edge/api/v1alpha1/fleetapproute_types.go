package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	RouteModeHub  = "hub"
	RouteModeSite = "site"
	RouteModeNone = "none"

	PhasePending  = "Pending"
	PhaseApplied  = "Applied"
	PhaseDegraded = "Degraded"
	PhaseDeleted  = "Deleted"

	FailoverModeManual      = "manual"
	FailoverModeRelocatable = "relocatable"
	FailoverModeStandby     = "standby"

	FailoverPhaseIdle       = "Idle"
	FailoverPhaseDegraded   = "Degraded"
	FailoverPhaseRelocating = "Relocating"
	FailoverPhaseFailed     = "Failed"

	ConditionHubRouteReachable = "HubRouteReachable"
	ConditionCertificateReady  = "CertificateReady"

	TLSPhasePendingDNS      = "PendingDNS"
	TLSPhasePendingPreflight = "PendingPreflight"
	TLSPhasePendingStaging  = "PendingStaging"
	TLSPhaseQueued          = "Queued"
	TLSPhaseIssuing         = "Issuing"
	TLSPhaseReady           = "Ready"
	TLSPhaseFailedDNS       = "FailedDNS"
	TLSPhaseFailedRateLimit = "FailedRateLimit"
	TLSPhaseFailed          = "Failed"
)

type ServiceRef struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Port      int32  `json:"port"`
}

// ClusterTarget is one placement provider (primary or standby) with its spoke Service.
type ClusterTarget struct {
	Cluster  string     `json:"cluster"`
	Provider string     `json:"provider,omitempty"`
	Role     string     `json:"role,omitempty"`
	SprmID   string     `json:"sprmId,omitempty"`
	Service  ServiceRef `json:"service"`
}

type FleetAppRouteSpec struct {
	InstanceID     string          `json:"instanceId"`
	CatalogItemID  string          `json:"catalogItemId,omitempty"`
	Cluster        string          `json:"cluster"`
	StandbyCluster string          `json:"standbyCluster,omitempty"`
	ClusterTargets []ClusterTarget `json:"clusterTargets,omitempty"`
	Mode           string          `json:"mode"`
	EffectiveMode  string          `json:"effectiveMode,omitempty"`
	HubHost        string          `json:"hubHost,omitempty"`
	SiteHost       string          `json:"siteHost,omitempty"`
	HealthPath     string          `json:"healthPath,omitempty"`
	FailoverMode   string          `json:"failoverMode,omitempty"`
	RequireAuth    bool            `json:"requireAuth,omitempty"`
	Service        ServiceRef      `json:"service"`
}

type FleetAppRouteStatus struct {
	Phase               string             `json:"phase,omitempty"`
	Message             string             `json:"message,omitempty"`
	TLSPhase            string             `json:"tlsPhase,omitempty"`
	TLSMessage          string             `json:"tlsMessage,omitempty"`
	HubPathHealthy      bool               `json:"hubPathHealthy,omitempty"`
	LastProbeAt         *metav1.Time       `json:"lastProbeAt,omitempty"`
	ConsecutiveFailures int                `json:"consecutiveFailures,omitempty"`
	ConsecutiveSuccess  int                `json:"consecutiveSuccess,omitempty"`
	ActiveCluster       string             `json:"activeCluster,omitempty"`
	FailoverPhase       string             `json:"failoverPhase,omitempty"`
	Conditions          []metav1.Condition `json:"conditions,omitempty"`
}

type FleetAppRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FleetAppRouteSpec   `json:"spec,omitempty"`
	Status            FleetAppRouteStatus `json:"status,omitempty"`
}

type FleetAppRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetAppRoute `json:"items"`
}

func (r *FleetAppRoute) EffectiveHealthPath() string {
	if r.Spec.HealthPath != "" {
		return r.Spec.HealthPath
	}
	return "/"
}

func (r *FleetAppRoute) ActiveClusterName() string {
	if r.Status.ActiveCluster != "" {
		return r.Status.ActiveCluster
	}
	return r.Spec.Cluster
}

// PlacementClusters returns per-provider targets or a single primary from spec.service.
func (r *FleetAppRoute) PlacementClusters() []ClusterTarget {
	if len(r.Spec.ClusterTargets) > 0 {
		return r.Spec.ClusterTargets
	}
	if r.Spec.Cluster == "" || r.Spec.Service.Name == "" {
		return nil
	}
	return []ClusterTarget{{
		Cluster: r.Spec.Cluster,
		Role:    "primary",
		Service: r.Spec.Service,
	}}
}

// ClusterNamesForCleanup returns all cluster namespaces that may hold a route ManifestWork.
func (r *FleetAppRoute) ClusterNamesForCleanup() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, t := range r.PlacementClusters() {
		if t.Cluster == "" {
			continue
		}
		if _, ok := seen[t.Cluster]; ok {
			continue
		}
		seen[t.Cluster] = struct{}{}
		out = append(out, t.Cluster)
	}
	if r.Spec.StandbyCluster != "" {
		if _, ok := seen[r.Spec.StandbyCluster]; !ok {
			out = append(out, r.Spec.StandbyCluster)
		}
	}
	if r.Spec.Cluster != "" {
		if _, ok := seen[r.Spec.Cluster]; !ok {
			out = append(out, r.Spec.Cluster)
		}
	}
	return out
}

func init() {
	SchemeBuilder.Register(&FleetAppRoute{}, &FleetAppRouteList{})
}
