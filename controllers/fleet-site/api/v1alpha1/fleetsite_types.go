package v1alpha1

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ProvisioningModeGreenfield = "greenfield"
	ProvisioningModeRegistry   = "registry"

	ExposureModeNone          = "none"
	ExposureModePortForward   = "port_forward"
	ExposureModeReverseProxy  = "reverse_proxy"
	ExposureModeHubGateway    = "hub_gateway"

	TLSPolicyInherit      = "inherit"
	TLSPolicyExternal     = "external"
	TLSPolicyACMECluster  = "acme_cluster"
	TLSPolicyACMEHub      = "acme_hub"
	TLSPolicyPassthrough  = "passthrough"
)

type FleetSiteSpec struct {
	SiteName string `json:"siteName"`
	// SiteId is the datacenter id for fleet.site OCM label (defaults to siteName).
	SiteId string `json:"siteId,omitempty"`
	OrgId            string `json:"orgId,omitempty"`
	// ProvisioningMode: greenfield (bootstrap Jobs) or registry (datacenter metadata only).
	ProvisioningMode string `json:"provisioningMode,omitempty"`
	DisplayName      string `json:"displayName,omitempty"`
	// Region is geographic / logical placement metadata for the datacenter.
	Region string `json:"region,omitempty"`
	// FailureDomain is an optional finer failure domain within region.
	FailureDomain string `json:"failureDomain,omitempty"`
	BootstrapHost    string `json:"bootstrapHost,omitempty"`
	MgmtLanIP        string `json:"mgmtLanIp,omitempty"`
	RockyVersion     string `json:"rockyVersion,omitempty"`
	ClusterRegion    string `json:"clusterRegion,omitempty"`
	MinRootDiskGi    int    `json:"minRootDiskGi,omitempty"`
	// Deprecated: cluster capabilities belong on ManagedCluster / FleetClusterJoin only.
	ClusterCapabilities map[string]bool `json:"clusterCapabilities,omitempty"`
	GPU              bool   `json:"gpu,omitempty"`
	Storage          *ClusterStorageSpec `json:"storage,omitempty"`
	Edge             FleetSiteEdgeSpec     `json:"edge,omitempty"`
	// EdgeDomain is deprecated; use edge.edgeDomain.
	EdgeDomain string `json:"edgeDomain,omitempty"`
}

type FleetSiteEdgeSpec struct {
	EdgeDomain           string                         `json:"edgeDomain,omitempty"`
	Wildcard             string                         `json:"wildcard,omitempty"`
	ExposureMode         string                         `json:"exposureMode,omitempty"`
	DefaultExposureMode  string                         `json:"defaultExposureMode,omitempty"`
	ExternalEndpoints    []FleetSiteExternalEndpointSpec `json:"externalEndpoints,omitempty"`
	Domains              []FleetSiteDomainSpec          `json:"domains,omitempty"`
	WanEgress            *FleetSiteWanEgressSpec        `json:"wanEgress,omitempty"`
	// Deprecated: use edge.wanEgress (site internet path, not cluster ingress).
	ExternalAccess       *FleetSiteExternalAccessSpec   `json:"externalAccess,omitempty"`
}

type FleetSiteExternalEndpointSpec struct {
	ID          string `json:"id"`
	Address     string `json:"address"`
	Role        string `json:"role,omitempty"`
	Description string `json:"description,omitempty"`
}

type FleetSiteDomainSpec struct {
	ID                string   `json:"id"`
	Zone              string   `json:"zone"`
	Wildcard          string   `json:"wildcard,omitempty"`
	TlsPolicy         string   `json:"tlsPolicy,omitempty"`
	AssignedClusters  []string `json:"assignedClusters,omitempty"`
	IngressEndpointId string   `json:"ingressEndpointId,omitempty"`
}

type FleetSiteExternalAccessSpec struct {
	Enabled       bool   `json:"enabled,omitempty"`
	DnsTargetNote string `json:"dnsTargetNote,omitempty"`
	ExpectedWanIp string `json:"expectedWanIp,omitempty"`
}

// FleetSiteWanEgressSpec — site has an internet egress path (not “all clusters are public”).
type FleetSiteWanEgressSpec struct {
	Enabled       bool   `json:"enabled,omitempty"`
	DnsTargetNote string `json:"dnsTargetNote,omitempty"`
	ExpectedWanIp string `json:"expectedWanIp,omitempty"`
}

const (
	StorageBackendLocalPath   = "local_path"
	StorageBackendNFSExternal = "nfs_external"
	// StorageBackendNFSLocal installs nfs-server on the single cluster node and
	// exports a local path for RWX (no external NFS server required).
	StorageBackendNFSLocal = "nfs_local"
	StorageBackendLonghorn = "longhorn"
)

// LocalPathStorageSpec toggles K3s default local-path provisioner (implicit when enabled).
type LocalPathStorageSpec struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// ClusterNFSSpec configures per-cluster NFS export subdirectory on a shared server.
type ClusterNFSSpec struct {
	Enabled      bool   `json:"enabled,omitempty"`
	Server       string `json:"server,omitempty"`
	ExportBase   string `json:"exportBase,omitempty"`
	ClusterPath  string `json:"clusterPath,omitempty"`
	StorageClass string `json:"storageClass,omitempty"`
	// DelegateHost is an SSH-reachable host (IP or DNS) used to create the export subdirectory on the NFS server.
	DelegateHost string `json:"delegateHost,omitempty"`
	// MountPath is the NFS client mount path (e.g. /srv/fleet-nfs/virt02). When empty, resolved at configure time (may use NFSv4 /cluster pseudo-path).
	MountPath string `json:"mountPath,omitempty"`
}

// ClusterLonghornSpec configures Longhorn storage (HA bootstrap).
// When Device is empty, Longhorn uses a directory on the root filesystem
// (defaultDataPath /var/lib/longhorn) instead of a dedicated block device.
type ClusterLonghornSpec struct {
	Enabled      bool   `json:"enabled,omitempty"`
	Device       string `json:"device,omitempty"`
	StorageClass string `json:"storageClass,omitempty"`
}

// ClusterStorageSpec describes cluster storage intent at provision time.
type ClusterStorageSpec struct {
	LocalPath     *LocalPathStorageSpec `json:"localPath,omitempty"`
	Backend       string                `json:"backend,omitempty"`
	MinDataDiskGi int                   `json:"minDataDiskGi,omitempty"`
	Rwo           bool                  `json:"rwo,omitempty"`
	Rwx           bool                  `json:"rwx,omitempty"`
	NFS           *ClusterNFSSpec       `json:"nfs,omitempty"`
	Longhorn      *ClusterLonghornSpec  `json:"longhorn,omitempty"`
}

// LonghornEnabled reports whether Longhorn install is requested.
func (s *ClusterStorageSpec) LonghornEnabled() bool {
	if s == nil {
		return false
	}
	if s.Longhorn != nil && s.Longhorn.Enabled {
		return true
	}
	return s.Backend == StorageBackendLonghorn
}

// LocalPathEnabled reports whether local-path is requested (default true).
func (s *ClusterStorageSpec) LocalPathEnabled() bool {
	if s != nil && s.LonghornEnabled() {
		if s.LocalPath == nil || s.LocalPath.Enabled == nil {
			return false
		}
		return *s.LocalPath.Enabled
	}
	if s == nil || s.LocalPath == nil || s.LocalPath.Enabled == nil {
		return true
	}
	return *s.LocalPath.Enabled
}

// NFSLocalEnabled reports whether single-node local NFS (node as server) is requested.
func (s *ClusterStorageSpec) NFSLocalEnabled() bool {
	if s == nil {
		return false
	}
	return s.Backend == StorageBackendNFSLocal
}

// NFSExternalEnabled reports whether external NFS install is requested.
func (s *ClusterStorageSpec) NFSExternalEnabled() bool {
	if s == nil {
		return false
	}
	if s.NFSLocalEnabled() {
		return false
	}
	if s.NFS != nil && s.NFS.Enabled {
		return true
	}
	return s.Backend == StorageBackendNFSExternal
}

// NFSEnabled reports whether any NFS provisioner install is requested (external or local).
func (s *ClusterStorageSpec) NFSEnabled() bool {
	if s == nil {
		return false
	}
	return s.NFSLocalEnabled() || s.NFSExternalEnabled()
}

// EffectiveBackend returns storage backend for ansible compatibility.
func (s *ClusterStorageSpec) EffectiveBackend() string {
	if s != nil && s.LonghornEnabled() {
		return StorageBackendLonghorn
	}
	if s != nil && s.NFSLocalEnabled() {
		return StorageBackendNFSLocal
	}
	if s != nil && s.NFSExternalEnabled() {
		return StorageBackendNFSExternal
	}
	return StorageBackendLocalPath
}

// EffectiveLonghornDevice returns the block device path for Longhorn data disks.
// Empty means directory mode on the root filesystem (no dedicated disk).
func (s *ClusterStorageSpec) EffectiveLonghornDevice() string {
	if s == nil || s.Longhorn == nil {
		return ""
	}
	return strings.TrimSpace(s.Longhorn.Device)
}

// EffectiveLonghornStorageClass returns the Longhorn StorageClass name.
func (s *ClusterStorageSpec) EffectiveLonghornStorageClass() string {
	if s == nil || s.Longhorn == nil || strings.TrimSpace(s.Longhorn.StorageClass) == "" {
		return "longhorn"
	}
	return strings.TrimSpace(s.Longhorn.StorageClass)
}

// FleetSiteStorageSpec is deprecated; use ClusterStorageSpec.
type FleetSiteStorageSpec = ClusterStorageSpec

// EffectiveWanEgress returns site WAN egress config (wanEgress preferred over legacy externalAccess).
func (s *FleetSiteSpec) EffectiveWanEgress() *FleetSiteWanEgressSpec {
	if s.Edge.WanEgress != nil {
		return s.Edge.WanEgress
	}
	if s.Edge.ExternalAccess != nil {
		return &FleetSiteWanEgressSpec{
			Enabled:       s.Edge.ExternalAccess.Enabled,
			DnsTargetNote: s.Edge.ExternalAccess.DnsTargetNote,
			ExpectedWanIp: s.Edge.ExternalAccess.ExpectedWanIp,
		}
	}
	return nil
}

type FleetSiteStatus struct {
	Phase                   string             `json:"phase,omitempty"`
	Message                 string             `json:"message,omitempty"`
	WireguardAddress        string             `json:"wireguardAddress,omitempty"`
	WireguardPublicKey      string             `json:"wireguardPublicKey,omitempty"`
	EnrolledAt              *metav1.Time       `json:"enrolledAt,omitempty"`
	EnrolledHostname        string             `json:"enrolledHostname,omitempty"`
	ValidationErrors        []string           `json:"validationErrors,omitempty"`
	ValidateJobName         string             `json:"validateJobName,omitempty"`
	ProvisionJobName        string             `json:"provisionJobName,omitempty"`
	ManagedClusterAvailable bool               `json:"managedClusterAvailable,omitempty"`
	Conditions              []metav1.Condition `json:"conditions,omitempty"`
}

// EffectiveEdgeDomain returns primary domain zone, legacy edgeDomain, or first domains[] entry.
func (s *FleetSiteSpec) EffectiveEdgeDomain() string {
	if s.Edge.EdgeDomain != "" {
		return s.Edge.EdgeDomain
	}
	if s.EdgeDomain != "" {
		return s.EdgeDomain
	}
	for _, d := range s.Edge.Domains {
		if d.Zone != "" {
			return d.Zone
		}
	}
	return ""
}

// EffectiveExposureMode returns site exposure mode (not inferred from wan egress).
func (s *FleetSiteSpec) EffectiveExposureMode() string {
	if s.Edge.ExposureMode != "" {
		return s.Edge.ExposureMode
	}
	return ExposureModeNone
}

// EffectiveDefaultExposureMode is the default for clusters with exposure_mode inherit.
func (s *FleetSiteSpec) EffectiveDefaultExposureMode() string {
	if s.Edge.DefaultExposureMode != "" {
		return s.Edge.DefaultExposureMode
	}
	return s.EffectiveExposureMode()
}

// EffectiveWildcard returns wildcard for the primary site zone.
func (s *FleetSiteSpec) EffectiveWildcard() string {
	if s.Edge.Wildcard != "" {
		return s.Edge.Wildcard
	}
	for _, d := range s.Edge.Domains {
		if d.Wildcard != "" {
			return d.Wildcard
		}
		if d.Zone != "" {
			return "*." + d.Zone
		}
	}
	zone := s.EffectiveEdgeDomain()
	if zone != "" {
		return "*." + zone
	}
	return ""
}

// NormalizeEdge fills domains[] from legacy edgeDomain when empty.
func (s *FleetSiteSpec) NormalizeEdge() {
	zone := s.EffectiveEdgeDomain()
	if zone == "" {
		return
	}
	if len(s.Edge.Domains) == 0 {
		s.Edge.Domains = []FleetSiteDomainSpec{{
			ID:       "primary",
			Zone:     zone,
			Wildcard: s.EffectiveWildcard(),
			TlsPolicy: defaultTlsForExposure(s.EffectiveExposureMode()),
		}}
	}
	if s.Edge.EdgeDomain == "" {
		s.Edge.EdgeDomain = zone
	}
	if s.Edge.Wildcard == "" {
		s.Edge.Wildcard = s.EffectiveWildcard()
	}
}

func defaultTlsForExposure(mode string) string {
	switch mode {
	case ExposureModeReverseProxy:
		return TLSPolicyExternal
	case ExposureModePortForward:
		return TLSPolicyACMECluster
	case ExposureModeHubGateway:
		return TLSPolicyACMEHub
	default:
		return TLSPolicyInherit
	}
}

// EffectiveSiteID returns siteId or siteName or metadata name fallback.
func (s *FleetSiteSpec) EffectiveSiteID(fallbackName string) string {
	if s.SiteId != "" {
		return s.SiteId
	}
	if s.SiteName != "" {
		return s.SiteName
	}
	return fallbackName
}

// IsRegistry returns true when this FleetSite is inventory/datacenter metadata only.
func (s *FleetSiteSpec) IsRegistry() bool {
	return s.ProvisioningMode == ProvisioningModeRegistry
}
