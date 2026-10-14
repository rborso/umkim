package v1alpha1

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ClusterTopologySingleNode = "single_node"
	ClusterTopologyHA3Node    = "ha_3_node"

	NetworkModeWireguard = "wireguard"
	NetworkModeLAN       = "lan"

	JoinPhasePending      = "Pending"
	JoinPhaseScriptIssued = "ScriptIssued"
	JoinPhaseEnrolled     = "Enrolled"
	JoinPhaseValidated    = "Validated"
	JoinPhaseProvisioning = "Provisioning"
	JoinPhaseReady        = "Ready"
	JoinPhaseFailed       = "Failed"
	JoinPhaseDeleting     = "Deleting"

	StoragePhasePending  = "Pending"
	StoragePhaseApplying = "Applying"
	StoragePhaseApplied  = "Applied"
	StoragePhaseFailed   = "Failed"

	// VmCleanup phases for the optional destroy of a cluster's YAIDAP VM guests
	// during cluster delete.
	VmCleanupPhaseDestroying = "Destroying"
	VmCleanupPhaseDone       = "Done"
	VmCleanupPhaseFailed     = "Failed"
	VmCleanupPhaseSkipped    = "Skipped"

	// AppsCleanup phases for the optional uninstall of a cluster's applications
	// during cluster delete.
	AppsCleanupPhaseUninstalling = "Uninstalling"
	AppsCleanupPhaseDone         = "Done"
	AppsCleanupPhaseFailed       = "Failed"

	ClusterCleanupFinalizer = "fleet.umkim.com/cluster-cleanup"

	NodeRoleControl = "control"
	NodeRoleWorker  = "worker"

	ProviderKindK8s      = "k8s"
	ProviderKindHelm     = "helm"
	ProviderKindKubevirt = "kubevirt"

	ProviderPhaseNotInstalled = "NotInstalled"
	ProviderPhaseInstalling   = "Installing"
	ProviderPhaseReady        = "Ready"
	ProviderPhaseDeleting     = "Deleting"
	ProviderPhaseFailed       = "Failed"

	ProviderK8sNodePortBase        = 30190
	ProviderHelmNodePortBase       = 30194
	ProviderKubevirtSPNodePortBase = 30220

	NodeOpTypeAddWorker           = "addWorker"
	NodeOpTypeRemoveWorker        = "removeWorker"
	NodeOpTypeReplaceControlPlane = "replaceControlPlane"
	// NodeOpTypeConfigureStorage is a cluster-level storage configure job (nfs_local / nfs_external).
	NodeOpTypeConfigureStorage = "configureStorage"
	// NodeOpTypeConfigureKubevirt is a cluster-level KubeVirt platform install job.
	NodeOpTypeConfigureKubevirt = "configureKubevirt"
	// NodeOpTypeConfigureGpuHost installs the NVIDIA host driver on cluster nodes.
	NodeOpTypeConfigureGpuHost = "configureGpuHost"
	// NodeOpTypeConfigureGpuEnable installs container toolkit and device plugin.
	NodeOpTypeConfigureGpuEnable = "configureGpuEnable"

	GpuHostPhaseNotPrepared = "NotPrepared"
	GpuHostPhaseApplying    = "Applying"
	GpuHostPhaseReady       = "Ready"
	GpuHostPhaseFailed      = "Failed"

	GpuRuntimePhaseNotEnabled = "NotEnabled"
	GpuRuntimePhaseApplying   = "Applying"
	GpuRuntimePhaseReady      = "Ready"
	GpuRuntimePhaseFailed     = "Failed"

	NodeOpPhasePending   = "Pending"
	NodeOpPhaseRunning   = "Running"
	NodeOpPhaseSucceeded = "Succeeded"
	NodeOpPhaseFailed    = "Failed"
)

// ProviderInstallOrder is the auto-install sequence after cluster Ready.
var ProviderInstallOrder = []string{
	ProviderKindKubevirt,
	ProviderKindK8s,
	ProviderKindHelm,
}

type FleetClusterJoinEdgeSpec struct {
	IngressId string `json:"ingressId,omitempty"`
}

type FleetClusterJoinNodeSpec struct {
	Hostname string            `json:"hostname,omitempty"`
	Address  string            `json:"address"`
	Role     string            `json:"role,omitempty"`
	GPU      bool              `json:"gpu,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	// VM asks the autoProvision pipeline to declare (or reuse) a YAIDAP VM guest
	// for this node and then create it. Nil means the node is not managed by the
	// pipeline: an existing inventory host is used, or the node is external/bare
	// metal. Zero resource fields fall back to the site catalog defaults.
	VM *FleetClusterJoinNodeVM `json:"vm,omitempty"`
}

// FleetClusterJoinNodeVM is the optional VM spec for a node the bring-up
// pipeline should declare and create. The guest name is the node hostname and
// the guest address is the node address, so no extra naming input is needed.
type FleetClusterJoinNodeVM struct {
	Cores      int    `json:"cores,omitempty"`
	Memory     int    `json:"memory,omitempty"`
	DiskGi     int    `json:"diskGi,omitempty"`
	DataDiskGi int    `json:"dataDiskGi,omitempty"`
	Hypervisor string `json:"hypervisor,omitempty"`
}

func (n FleetClusterJoinNodeSpec) EffectiveRole() string {
	if n.Role == "" || n.Role == NodeRoleControl {
		return NodeRoleControl
	}
	return n.Role
}

type ProviderInstallSpec struct {
	K8s      bool `json:"k8s,omitempty"`
	Helm     bool `json:"helm,omitempty"`
	Kubevirt bool `json:"kubevirt,omitempty"`
}

// SetIntent records whether a provider kind should be auto-installed after Ready.
// Manual delete clears the flag so reconcile does not immediately reinstall.
func (p *ProviderInstallSpec) SetIntent(kind string, install bool) {
	switch kind {
	case ProviderKindK8s:
		p.K8s = install
	case ProviderKindHelm:
		p.Helm = install
	case ProviderKindKubevirt:
		p.Kubevirt = install
	}
}

// KubevirtBridgeSpec configures day-0 host Linux bridge for Multus L2 VMs.
// Applied by the bootstrap bundle shell script on all nodes when Enabled.
type KubevirtBridgeSpec struct {
	// Enabled creates/ensures the host bridge during bootstrap (requires install.kubevirt).
	Enabled bool `json:"enabled,omitempty"`
	// BridgeName is the Linux bridge device name (default mgmt-l2-br).
	BridgeName string `json:"bridgeName,omitempty"`
	// SlaveNic is the physical NIC to enslave; empty = default-route interface.
	SlaveNic string `json:"slaveNic,omitempty"`
	// KeepIP moves the slave NIC's IPv4 address onto the bridge (default true when Enabled).
	KeepIP *bool `json:"keepIp,omitempty"`
}

func (k KubevirtBridgeSpec) EffectiveBridgeName() string {
	if strings.TrimSpace(k.BridgeName) != "" {
		return strings.TrimSpace(k.BridgeName)
	}
	return "mgmt-l2-br"
}

func (k KubevirtBridgeSpec) EffectiveKeepIP() bool {
	if k.KeepIP == nil {
		return true
	}
	return *k.KeepIP
}

type ClusterProvidersSpec struct {
	K8sNodePort        int                 `json:"k8sNodePort,omitempty"`
	HelmNodePort       int                 `json:"helmNodePort,omitempty"`
	KubevirtSPNodeport int                 `json:"kubevirtSpNodeport,omitempty"`
	Install            ProviderInstallSpec `json:"install,omitempty"`
	// KubevirtBridge hosts Multus bridge VMs on the LAN (NAD applied at kubevirt provider install).
	KubevirtBridge KubevirtBridgeSpec `json:"kubevirtBridge,omitempty"`
}

// WantsKubevirtBridge reports whether bootstrap should configure a host bridge.
func (s *FleetClusterJoinSpec) WantsKubevirtBridge() bool {
	return s.Providers.Install.Kubevirt && s.Providers.KubevirtBridge.Enabled
}

type ClusterProviderStatus struct {
	Phase        string `json:"phase,omitempty"`
	JobName      string `json:"jobName,omitempty"`
	Message      string `json:"message,omitempty"`
	ProviderName string `json:"providerName,omitempty"`
}

type ClusterGpuStatus struct {
	HostPhase      string `json:"hostPhase,omitempty"`
	HostJobName    string `json:"hostJobName,omitempty"`
	HostMessage    string `json:"hostMessage,omitempty"`
	RuntimePhase   string `json:"runtimePhase,omitempty"`
	RuntimeJobName string `json:"runtimeJobName,omitempty"`
	RuntimeMessage string `json:"runtimeMessage,omitempty"`
	PciID          string `json:"pciId,omitempty"`
	MarketingName  string `json:"marketingName,omitempty"`
	DriverBranch   string `json:"driverBranch,omitempty"`
	DriverLoaded   bool   `json:"driverLoaded,omitempty"`
}

type ClusterStorageStatus struct {
	Phase   string `json:"phase,omitempty"`
	JobName string `json:"jobName,omitempty"`
	Message string `json:"message,omitempty"`
}

type ClusterNodeOperationStatus struct {
	ID        string       `json:"id,omitempty"`
	Type      string       `json:"type,omitempty"`
	Node      string       `json:"node,omitempty"`
	Phase     string       `json:"phase,omitempty"`
	JobName   string       `json:"jobName,omitempty"`
	Message   string       `json:"message,omitempty"`
	Force     bool         `json:"force,omitempty"`
	ActorKind string       `json:"actorKind,omitempty"`
	ActorName string       `json:"actorName,omitempty"`
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
}

// ClusterNodeOperationRecord is an append-only history entry for a completed node operation.
type ClusterNodeOperationRecord struct {
	ID         string       `json:"id,omitempty"`
	Type       string       `json:"type,omitempty"`
	Node       string       `json:"node,omitempty"`
	Phase      string       `json:"phase,omitempty"`
	JobName    string       `json:"jobName,omitempty"`
	Message    string       `json:"message,omitempty"`
	Force      bool         `json:"force,omitempty"`
	ActorKind  string       `json:"actorKind,omitempty"`
	ActorName  string       `json:"actorName,omitempty"`
	StartedAt  *metav1.Time `json:"startedAt,omitempty"`
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`
	LogTail    string       `json:"logTail,omitempty"`
}

type FleetClusterJoinNodeStatus struct {
	Hostname           string `json:"hostname,omitempty"`
	Address            string `json:"address,omitempty"`
	WireguardAddress   string `json:"wireguardAddress,omitempty"`
	WireguardPublicKey string `json:"wireguardPublicKey,omitempty"`
}

type FleetClusterJoinSpec struct {
	ClusterName         string                    `json:"clusterName"`
	SiteId              string                    `json:"siteId"`
	BootstrapHost       string                    `json:"bootstrapHost"`
	Jumphost            string                    `json:"jumphost,omitempty"`
	MinRootDiskGi       int                       `json:"minRootDiskGi,omitempty"`
	RockyVersion        string                    `json:"rockyVersion,omitempty"`
	ClusterRegion       string                    `json:"clusterRegion,omitempty"`
	ClusterLabels       map[string]string         `json:"clusterLabels,omitempty"`
	ClusterCapabilities map[string]bool           `json:"clusterCapabilities,omitempty"`
	GPU                 bool                      `json:"gpu,omitempty"`
	Storage             *ClusterStorageSpec       `json:"storage,omitempty"`
	Edge                FleetClusterJoinEdgeSpec  `json:"edge,omitempty"`
	Topology            string                    `json:"topology,omitempty"`
	NetworkMode         string                    `json:"networkMode,omitempty"`
	HubLanApi           string                    `json:"hubLanApi,omitempty"`
	ControlPlaneVip     string                    `json:"controlPlaneVip,omitempty"`
	Nodes               []FleetClusterJoinNodeSpec `json:"nodes,omitempty"`
	Providers           ClusterProvidersSpec       `json:"providers,omitempty"`
	IngressAuth         FleetClusterJoinIngressAuthSpec `json:"ingressAuth,omitempty"`
	Acme                FleetClusterJoinAcmeSpec   `json:"acme,omitempty"`
	// AutoProvision drives the full bring-up pipeline in the controller: create
	// Declared VM guests, run the bootstrap bundle via the jumphost, then provision
	// the hub side. When false the intent is created idle (operator runs the steps).
	AutoProvision bool `json:"autoProvision,omitempty"`
}

type FleetClusterJoinStatus struct {
	Phase                   string                       `json:"phase,omitempty"`
	Message                 string                       `json:"message,omitempty"`
	WireguardAddress        string                       `json:"wireguardAddress,omitempty"`
	WireguardPublicKey      string                       `json:"wireguardPublicKey,omitempty"`
	Nodes                   []FleetClusterJoinNodeStatus `json:"nodes,omitempty"`
	EnrolledAt              *metav1.Time                 `json:"enrolledAt,omitempty"`
	EnrolledHostname        string       `json:"enrolledHostname,omitempty"`
	ValidationErrors        []string     `json:"validationErrors,omitempty"`
	ValidateJobName         string       `json:"validateJobName,omitempty"`
	BootstrapJobName        string       `json:"bootstrapJobName,omitempty"`
	BootstrapActionID       string       `json:"bootstrapActionID,omitempty"`
	PipelineStep            string       `json:"pipelineStep,omitempty"`
	ProvisionJobName        string       `json:"provisionJobName,omitempty"`
	ProvisionActionID       string       `json:"provisionActionID,omitempty"`
	DeleteJobName           string       `json:"deleteJobName,omitempty"`
	WgRemoveJobName         string       `json:"wgRemoveJobName,omitempty"`
	DeletionStartedAt       *metav1.Time `json:"deletionStartedAt,omitempty"`
	ManagedClusterAvailable bool                            `json:"managedClusterAvailable,omitempty"`
	Providers               map[string]ClusterProviderStatus `json:"providers,omitempty"`
	Gpu                     ClusterGpuStatus                 `json:"gpu,omitempty"`
	Storage                 ClusterStorageStatus             `json:"storage,omitempty"`
	IngressAuth             FleetClusterJoinIngressAuthStatus `json:"ingressAuth,omitempty"`
	Acme                    FleetClusterJoinAcmeStatus        `json:"acme,omitempty"`
	NodeOperation           *ClusterNodeOperationStatus       `json:"nodeOperation,omitempty"`
	NodeOperationHistory    []ClusterNodeOperationRecord      `json:"nodeOperationHistory,omitempty"`
	VmCleanup               *ClusterVmCleanupStatus           `json:"vmCleanup,omitempty"`
	AppsCleanup             *ClusterAppsCleanupStatus         `json:"appsCleanup,omitempty"`
}

// ClusterAppsCleanupStatus tracks the optional uninstall of a cluster's
// applications during cluster delete. The apps are deleted through their own
// finalizer (which runs the Helm uninstall on the spoke and drops the route and
// OIDC client), so Apps accumulates the work list and Completed the apps that
// are gone.
type ClusterAppsCleanupStatus struct {
	Phase     string       `json:"phase,omitempty"`
	Message   string       `json:"message,omitempty"`
	Apps      []string     `json:"apps,omitempty"`
	Completed []string     `json:"completed,omitempty"`
	Failures  []string     `json:"failures,omitempty"`
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
}

// ClusterVmCleanupStatus tracks the optional destroy of a cluster's YAIDAP VM
// guests — and, when asked, removal of their site inventory declarations — as
// the final stage of cluster delete. Hosts is the stable, ordered work list
// derived from the cluster nodes; Completed accumulates finished hosts so a
// requeue resumes instead of restarting.
type ClusterVmCleanupStatus struct {
	Phase     string   `json:"phase,omitempty"`
	Message   string   `json:"message,omitempty"`
	Hosts     []string `json:"hosts,omitempty"`
	Completed []string `json:"completed,omitempty"`
	ActionID  string   `json:"actionID,omitempty"`
	JobName   string   `json:"jobName,omitempty"`
	Failures  []string `json:"failures,omitempty"`
}

func (s *FleetClusterJoinSpec) K8sProviderName(clusterName string) string {
	return "k8s-" + clusterName
}

func (s *FleetClusterJoinSpec) HelmProviderName(clusterName string) string {
	return "helm-" + clusterName
}

func (s *FleetClusterJoinSpec) KubevirtProviderName(clusterName string) string {
	return "kubevirt-" + clusterName
}

func (s *FleetClusterJoinSpec) WantsProviderInstall(kind string) bool {
	switch kind {
	case ProviderKindK8s:
		return s.Providers.Install.K8s
	case ProviderKindHelm:
		return s.Providers.Install.Helm
	case ProviderKindKubevirt:
		return s.Providers.Install.Kubevirt
	default:
		return false
	}
}

func providerStatusPhase(st ClusterProviderStatus) string {
	if st.Phase == "" {
		return ProviderPhaseNotInstalled
	}
	return st.Phase
}

func (s *FleetClusterJoinSpec) EffectiveClusterName(fallback string) string {
	if s.ClusterName != "" {
		return s.ClusterName
	}
	return fallback
}

func (s *FleetClusterJoinSpec) IsHA3Node() bool {
	return s.Topology == ClusterTopologyHA3Node
}

func (s *FleetClusterJoinSpec) IsLAN() bool {
	return s.NetworkMode == NetworkModeLAN
}

func (s *FleetClusterJoinSpec) EffectiveHubLanAPI(fallback string) string {
	if s.HubLanApi != "" {
		return s.HubLanApi
	}
	return fallback
}

func (s *FleetClusterJoinSpec) InitNodeHostname(clusterName string) string {
	for _, n := range s.Nodes {
		if n.EffectiveRole() != NodeRoleControl {
			continue
		}
		if n.Hostname != "" {
			return n.Hostname
		}
		return clusterName + "-1"
	}
	if len(s.Nodes) > 0 && s.Nodes[0].Hostname != "" {
		return s.Nodes[0].Hostname
	}
	return clusterName + "-1"
}

func (s *FleetClusterJoinSpec) ControlNodes() []FleetClusterJoinNodeSpec {
	out := make([]FleetClusterJoinNodeSpec, 0)
	for _, n := range s.Nodes {
		if n.EffectiveRole() == NodeRoleControl {
			out = append(out, n)
		}
	}
	return out
}

func (s *FleetClusterJoinSpec) WorkerNodes() []FleetClusterJoinNodeSpec {
	out := make([]FleetClusterJoinNodeSpec, 0)
	for _, n := range s.Nodes {
		if n.EffectiveRole() == NodeRoleWorker {
			out = append(out, n)
		}
	}
	return out
}

func (s *FleetClusterJoinSpec) Capabilities() map[string]bool {
	out := map[string]bool{}
	for k, v := range s.ClusterCapabilities {
		out[k] = v
	}
	if s.Providers.Install.Kubevirt {
		out["kvm"] = true
	}
	if s.IsHA3Node() {
		out["ha_topology"] = true
		out["ha_cluster"] = true
	}
	return out
}
