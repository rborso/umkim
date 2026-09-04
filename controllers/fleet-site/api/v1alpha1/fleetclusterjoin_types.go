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
	// BridgeName is the Linux bridge device name (default n0p0_mgmt_nw).
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
	return "n0p0_mgmt_nw"
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
	ProvisionJobName        string       `json:"provisionJobName,omitempty"`
	DeleteJobName           string       `json:"deleteJobName,omitempty"`
	WgRemoveJobName         string       `json:"wgRemoveJobName,omitempty"`
	DeletionStartedAt       *metav1.Time `json:"deletionStartedAt,omitempty"`
	ManagedClusterAvailable bool                            `json:"managedClusterAvailable,omitempty"`
	Providers               map[string]ClusterProviderStatus `json:"providers,omitempty"`
	Storage                 ClusterStorageStatus             `json:"storage,omitempty"`
	IngressAuth             FleetClusterJoinIngressAuthStatus `json:"ingressAuth,omitempty"`
	Acme                    FleetClusterJoinAcmeStatus        `json:"acme,omitempty"`
	NodeOperation           *ClusterNodeOperationStatus       `json:"nodeOperation,omitempty"`
	NodeOperationHistory    []ClusterNodeOperationRecord      `json:"nodeOperationHistory,omitempty"`
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
