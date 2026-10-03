package v1alpha1

import (
	"net"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ProvisioningModeGreenfield = "greenfield"
	ProvisioningModeRegistry   = "registry"

	ExposureModeNone         = "none"
	ExposureModePortForward  = "port_forward"
	ExposureModeReverseProxy = "reverse_proxy"
	ExposureModeHubGateway   = "hub_gateway"

	IngressMethodPortForward  = "port_forward"
	IngressMethodReverseProxy = "reverse_proxy"

	WanAddressModeFixed = "fixed"
	WanAddressModeDNS   = "dns"

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
	// ProvisioningMode: greenfield (bootstrap Jobs) or registry (datacenter metadata only).
	ProvisioningMode string `json:"provisioningMode,omitempty"`
	DisplayName      string `json:"displayName,omitempty"`
	// Region is geographic / logical placement metadata for the datacenter.
	Region string `json:"region,omitempty"`
	// FailureDomain is an optional finer failure domain within region.
	FailureDomain string `json:"failureDomain,omitempty"`
	BootstrapHost    string `json:"bootstrapHost,omitempty"`
	Jumphost         string `json:"jumphost,omitempty"`
	MgmtLanIP        string `json:"mgmtLanIp,omitempty"`
	RockyVersion     string `json:"rockyVersion,omitempty"`
	ClusterRegion    string `json:"clusterRegion,omitempty"`
	MinRootDiskGi    int    `json:"minRootDiskGi,omitempty"`
	// Deprecated: cluster capabilities belong on ManagedCluster / FleetClusterJoin only.
	ClusterCapabilities map[string]bool `json:"clusterCapabilities,omitempty"`
	GPU              bool   `json:"gpu,omitempty"`
	Storage          *ClusterStorageSpec `json:"storage,omitempty"`
	Edge FleetSiteEdgeSpec `json:"edge,omitempty"`
	// Provision binds an optional metal/VM provisioner connector (YAIDAP, tofu, …).
	// Default / empty is equivalent to provider "noop" (hosts already SSH-ready).
	Provision *SiteProvisionSpec `json:"provision,omitempty"`
}

// Provision connector providers (FleetSite.spec.provision.provider).
const (
	ProvisionProviderNoop   = "noop"
	ProvisionProviderYaidap = "yaidap"
)

// Provision connector actions (POST …/provision/actions).
const (
	ProvisionActionInstallBM = "install_bm"
	ProvisionActionCreateVM  = "create_vm"
	ProvisionActionPlanVM    = "plan_vm"
	ProvisionActionDestroy   = "destroy"
	ProvisionActionDiscover    = "discover"
	ProvisionActionDiscoverPXE = "discover_pxe"
	ProvisionActionStatus      = "status"
	ProvisionActionPrepareHypervisor       = "prepare_hypervisor"
	ProvisionActionPrepareGPUPassthrough   = "prepare_gpu_passthrough"
	ProvisionActionPrepareHypervisorGPU    = "prepare_hypervisor_gpu"
)

// Provision action phases.
const (
	ProvisionPhasePending   = "Pending"
	ProvisionPhaseRunning   = "Running"
	ProvisionPhaseSucceeded = "Succeeded"
	ProvisionPhaseFailed    = "Failed"
)

// SiteProvisionSpec is the site-scoped provisioner connector binding.
type SiteProvisionSpec struct {
	// Provider: noop | yaidap
	Provider string `json:"provider,omitempty"`
	// Jumphost is the SSH target that runs YAIDAP / tofu (user@host:port).
	Jumphost string `json:"jumphost,omitempty"`
	// SSHSecretRef is a Secret in the controller namespace with an SSH private key for jumphost.
	SSHSecretRef string `json:"sshSecretRef,omitempty"`
	// InventoryPath is the absolute path on the jumphost to the YAIDAP inventory root.
	InventoryPath string `json:"inventoryPath,omitempty"`
	// YaidaPRoot is an optional path to the YAIDAP checkout on the jumphost.
	YaidaPRoot string `json:"yaidapRoot,omitempty"`
	// YaidapRepo is the git remote URL for hub-managed YAIDAP checkout sync (https or git@).
	YaidapRepo string `json:"yaidapRepo,omitempty"`
	// YaidapRef is the branch, tag, or commit to checkout (default main).
	YaidapRef string `json:"yaidapRef,omitempty"`
	// YaidapGitSecretRef is an optional hub Secret with a deploy key (id_ed25519, deploy_key, or id_rsa).
	YaidapGitSecretRef string `json:"yaidapGitSecretRef,omitempty"`
	// DefaultExtraVars are safe non-secret extras passed to the connector (no passwords).
	DefaultExtraVars map[string]string `json:"defaultExtraVars,omitempty"`
}

// ProvisionHandoffHost is the minimal SSH-ready host fragment returned on success.
type ProvisionHandoffHost struct {
	Hostname    string   `json:"hostname,omitempty"`
	AnsibleHost string   `json:"ansible_host,omitempty"`
	AnsibleUser string   `json:"ansible_user,omitempty"`
	OSFamily    string   `json:"osFamily,omitempty"`
	MACs        []string `json:"macs,omitempty"`
	SiteID      string   `json:"siteId,omitempty"`
}

// SiteProvisionActionRecord is one provision connector action (current or history).
type SiteProvisionActionRecord struct {
	ID        string                `json:"id"`
	Action    string                `json:"action"`
	Subject   string                `json:"subject,omitempty"`
	Provider  string                `json:"provider,omitempty"`
	Phase     string                `json:"phase,omitempty"`
	Message   string                `json:"message,omitempty"`
	Confirm   bool                  `json:"confirm,omitempty"`
	JobName   string                `json:"jobName,omitempty"`
	Host      *ProvisionHandoffHost `json:"host,omitempty"`
	ActorKind string                `json:"actorKind,omitempty"`
	ActorName string                `json:"actorName,omitempty"`
	LogTail   string                `json:"logTail,omitempty"`
	StartedAt *metav1.Time          `json:"startedAt,omitempty"`
	EndedAt   *metav1.Time          `json:"endedAt,omitempty"`
}

// SiteProvisionStatus tracks the last connector action and recent history.
type SiteProvisionStatus struct {
	Phase          string                      `json:"phase,omitempty"`
	Message        string                      `json:"message,omitempty"`
	LastActionID   string                      `json:"lastActionID,omitempty"`
	LastActionType string                      `json:"lastActionType,omitempty"`
	YaidapRevision string                      `json:"yaidapRevision,omitempty"`
	YaidapSyncedAt string                      `json:"yaidapSyncedAt,omitempty"`
	Current        *SiteProvisionActionRecord  `json:"current,omitempty"`
	History        []SiteProvisionActionRecord `json:"history,omitempty"`
}

// Site host inventory kinds (derived from YAIDAP inventory layout).
const (
	SiteHostKindVM         = "vm"          // guests.yml → guests.hosts (libvirt workloads)
	SiteHostKindBareMetal  = "bare_metal"  // bare_metals.yml (cluster nodes, not libvirt guests)
	SiteHostKindHypervisor = "hypervisor"  // hypervisors.yml (placement / discovery / TempXE targets)
)

// Site host inventory phases.
const (
	SiteHostPhaseDeclared     = "Declared"
	SiteHostPhaseProvisioning = "Provisioning"
	SiteHostPhaseDestroying   = "Destroying"
	SiteHostPhaseReady        = "Ready"
	SiteHostPhaseFailed       = "Failed"
)

// Site host cluster intent modes (applied after SSH-ready provision).
const (
	SiteHostClusterModeEnroll          = "enroll"
	SiteHostClusterModeAddWorker       = "add_worker"
	SiteHostClusterModeReplaceControl  = "replace_control"
)

// Site host cluster status phases (observed cluster handoff).
const (
	SiteHostClusterPhasePending      = "Pending"
	SiteHostClusterPhaseWaitingGroup = "WaitingGroup"
	SiteHostClusterPhaseApplying     = "Applying"
	SiteHostClusterPhaseReady        = "Ready"
	SiteHostClusterPhaseFailed       = "Failed"
)

// SiteHostClusterIntent declares optional post-provision cluster automation.
type SiteHostClusterIntent struct {
	Mode            string `json:"mode,omitempty"`
	ClusterName     string `json:"clusterName,omitempty"`
	TargetCluster   string `json:"targetCluster,omitempty"`
	Topology        string `json:"topology,omitempty"`
	HAGroup         string `json:"haGroup,omitempty"`
	ReplaceNode     string `json:"replaceNode,omitempty"`
	ControlPlaneVip string `json:"controlPlaneVip,omitempty"`
	Jumphost        string `json:"jumphost,omitempty"`
	IngressId       string `json:"ingressId,omitempty"`
	AutoApply       *bool  `json:"autoApply,omitempty"`
}

func (i *SiteHostClusterIntent) EffectiveAutoApply() bool {
	if i == nil || i.AutoApply == nil {
		return true
	}
	return *i.AutoApply
}

func (i *SiteHostClusterIntent) EffectiveTopology() string {
	if i == nil || strings.TrimSpace(i.Topology) == "" {
		return ClusterTopologySingleNode
	}
	return strings.TrimSpace(i.Topology)
}

// SiteHostClusterStatus tracks cluster handoff progress for a host.
type SiteHostClusterStatus struct {
	Phase       string `json:"phase,omitempty"`
	Message     string `json:"message,omitempty"`
	ClusterName string `json:"clusterName,omitempty"`
	FCJName     string `json:"fcjName,omitempty"`
	Mode        string `json:"mode,omitempty"`
}

// SiteHostRecord is a thin hub-owned host intent for provisioner connectors.
// Kind is inventory-derived: vm (guests.hosts), bare_metal (bare_metals.yml),
// hypervisor (hypervisors.yml).
type SiteHostRecord struct {
	Name          string                 `json:"name"`
	Kind          string                 `json:"kind,omitempty"`
	DesiredState  string                 `json:"desiredState,omitempty"`
	AnsibleHost   string                 `json:"ansible_host,omitempty"`
	AnsibleUser   string                 `json:"ansible_user,omitempty"`
	OSFamily      string                 `json:"osFamily,omitempty"`
	VmLocation    string                 `json:"vm_location,omitempty"`
	Flavor        map[string]string      `json:"flavor,omitempty"`
	Labels        map[string]string      `json:"labels,omitempty"`
	ClusterIntent *SiteHostClusterIntent `json:"clusterIntent,omitempty"`
	Status        SiteHostStatus         `json:"status,omitempty"`
}

// SiteHostStatus tracks observed provision state on a hub host record.
type SiteHostStatus struct {
	Phase           string                 `json:"phase,omitempty"`
	Message         string                 `json:"message,omitempty"`
	LastProvisionID string                 `json:"lastProvisionID,omitempty"`
	AnsibleHost     string                 `json:"ansible_host,omitempty"`
	Cluster         *SiteHostClusterStatus `json:"cluster,omitempty"`
}

type FleetSiteEdgeSpec struct {
	HubGateway  bool                    `json:"hubGateway,omitempty"`
	HubZone     string                  `json:"hubZone,omitempty"`
	HubWildcard string                  `json:"hubWildcard,omitempty"`
	Ingress     []FleetSiteIngressSpec  `json:"ingress,omitempty"`
	WanEgress   *FleetSiteWanEgressSpec `json:"wanEgress,omitempty"`
	// Deprecated: use edge.wanEgress (site internet path, not cluster ingress).
	ExternalAccess *FleetSiteExternalAccessSpec `json:"externalAccess,omitempty"`
}

// FleetSiteIngressSpec is one public ingress path at a site (port-forward or reverse-proxy).
type FleetSiteIngressSpec struct {
	ID               string   `json:"id"`
	Method           string   `json:"method"`
	Address          string   `json:"address,omitempty"`
	WanAddressMode   string   `json:"wanAddressMode,omitempty"`
	Zone             string   `json:"zone,omitempty"`
	Wildcard         string   `json:"wildcard,omitempty"`
	Description      string   `json:"description,omitempty"`
	OperatorNotes    string   `json:"operatorNotes,omitempty"`
	AssignedClusters []string `json:"assignedClusters,omitempty"`
}

func (i FleetSiteIngressSpec) EffectiveWildcard() string {
	if w := strings.TrimSpace(i.Wildcard); w != "" {
		return w
	}
	if z := strings.TrimSpace(i.Zone); z != "" {
		return "*." + z
	}
	return ""
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

func isPrivateAddress(addr string) bool {
	ip := net.ParseIP(strings.TrimSpace(addr))
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}

// EffectiveWanAddressMode resolves WAN validation mode for a port_forward ingress row.
// Explicit wanAddressMode wins; otherwise infer from wanEgress, private address, or default fixed.
func (s *FleetSiteSpec) EffectiveWanAddressMode(ent FleetSiteIngressSpec) string {
	mode := strings.TrimSpace(ent.WanAddressMode)
	switch mode {
	case WanAddressModeFixed, WanAddressModeDNS:
		return mode
	}
	if we := s.EffectiveWanEgress(); we != nil && we.Enabled && strings.TrimSpace(we.ExpectedWanIp) == "" {
		return WanAddressModeDNS
	}
	if isPrivateAddress(ent.Address) {
		return WanAddressModeDNS
	}
	return WanAddressModeFixed
}

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
	// Provision is optional metal/VM connector status (distinct from greenfield ProvisionJobName).
	Provision *SiteProvisionStatus `json:"provision,omitempty"`
}

// IngressByID returns a site ingress entry by id.
func (s *FleetSiteSpec) IngressByID(id string) (FleetSiteIngressSpec, bool) {
	id = strings.TrimSpace(id)
	s.NormalizeIngress()
	for _, ent := range s.Edge.Ingress {
		if ent.ID == id {
			return ent, true
		}
	}
	return FleetSiteIngressSpec{}, false
}

// NormalizeIngress fills default wildcards on port_forward ingress rows.
func (s *FleetSiteSpec) NormalizeIngress() {
	for i := range s.Edge.Ingress {
		ent := &s.Edge.Ingress[i]
		if ent.Wildcard == "" && ent.Zone != "" && ent.Method == IngressMethodPortForward {
			ent.Wildcard = "*." + ent.Zone
		}
	}
}

// AutoIngressID returns the sole port_forward ingress id when the site has exactly one ingress row.
func (s *FleetSiteSpec) AutoIngressID() string {
	s.NormalizeIngress()
	if len(s.Edge.Ingress) != 1 {
		return ""
	}
	if s.Edge.Ingress[0].Method == IngressMethodPortForward {
		return s.Edge.Ingress[0].ID
	}
	return ""
}

// EffectiveHubZone returns hub catalog DNS zone when hubGateway is enabled.
func (s *FleetSiteSpec) EffectiveHubZone() string {
	if z := strings.TrimSpace(s.Edge.HubZone); z != "" {
		return z
	}
	return ""
}

// EffectiveHubWildcard returns hub wildcard when hubGateway is enabled.
func (s *FleetSiteSpec) EffectiveHubWildcard() string {
	if w := strings.TrimSpace(s.Edge.HubWildcard); w != "" {
		return w
	}
	if z := s.EffectiveHubZone(); z != "" {
		return "*." + z
	}
	return ""
}

// EffectiveEdgeDomain returns the first port_forward ingress zone (app siteHost defaults).
func (s *FleetSiteSpec) EffectiveEdgeDomain() string {
	s.NormalizeIngress()
	for _, ent := range s.Edge.Ingress {
		if ent.Method == IngressMethodPortForward && ent.Zone != "" {
			return ent.Zone
		}
	}
	return s.EffectiveHubZone()
}

// EffectiveWildcard returns wildcard for the first port_forward ingress.
func (s *FleetSiteSpec) EffectiveWildcard() string {
	s.NormalizeIngress()
	for _, ent := range s.Edge.Ingress {
		if ent.Method == IngressMethodPortForward {
			if w := ent.EffectiveWildcard(); w != "" {
				return w
			}
		}
	}
	return s.EffectiveHubWildcard()
}

// HubGatewayEnabled reports whether hub-routed public URLs are configured for the site.
func (s *FleetSiteSpec) HubGatewayEnabled() bool {
	return s.Edge.HubGateway
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

// EffectiveProvisionProvider returns the connector provider (default noop).
func (s *FleetSiteSpec) EffectiveProvisionProvider() string {
	if s.Provision == nil || strings.TrimSpace(s.Provision.Provider) == "" {
		return ProvisionProviderNoop
	}
	return strings.TrimSpace(s.Provision.Provider)
}
