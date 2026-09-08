package clustercreate

import (
	"context"
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/config"
	"github.com/rborso/umkim/controllers/fleet-site/internal/ingress"
	"github.com/rborso/umkim/controllers/fleet-site/internal/providers"
	"github.com/rborso/umkim/controllers/fleet-site/internal/storage"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var managedClusterGVK = schema.GroupVersionKind{
	Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
}

// Request mirrors POST /clusters create body fields used for FleetClusterJoin creation.
type Request struct {
	ClusterName         string
	SiteId              string
	BootstrapHost       string
	Jumphost            string
	MinRootDiskGi       int
	ClusterRegion       string
	ClusterLabels       map[string]string
	ClusterCapabilities map[string]bool
	GPU                 bool
	Storage             *fleetv1alpha1.ClusterStorageSpec
	Edge                fleetv1alpha1.FleetClusterJoinEdgeSpec
	Topology            string
	NetworkMode         string
	RockyVersion        string
	HubLanApi           string
	ControlPlaneVip     string
	Nodes               []fleetv1alpha1.FleetClusterJoinNodeSpec
	Install             fleetv1alpha1.ProviderInstallSpec
	KubevirtBridge      fleetv1alpha1.KubevirtBridgeSpec
}

// Create validates and creates a FleetClusterJoin CR.
func Create(ctx context.Context, c client.Client, cfg config.Config, req Request) (*fleetv1alpha1.FleetClusterJoin, error) {
	if req.ClusterName == "" || req.SiteId == "" || req.BootstrapHost == "" {
		return nil, fmt.Errorf("clusterName, siteId, and bootstrapHost required")
	}
	topology := req.Topology
	if topology == "" {
		topology = fleetv1alpha1.ClusterTopologySingleNode
	}
	networkMode := req.NetworkMode
	if networkMode == "" {
		networkMode = fleetv1alpha1.NetworkModeWireguard
	}
	if networkMode == fleetv1alpha1.NetworkModeLAN {
		return nil, fmt.Errorf("networkMode lan is deprecated; use wireguard overlay")
	}
	nodes, err := NormalizeNodes(req.ClusterName, topology, req.BootstrapHost, req.Nodes)
	if err != nil {
		return nil, err
	}
	if topology == fleetv1alpha1.ClusterTopologySingleNode {
		if req.BootstrapHost != nodes[0].Address {
			return nil, fmt.Errorf("bootstrapHost must match the control node address")
		}
	}
	if topology == fleetv1alpha1.ClusterTopologyHA3Node {
		controls := 0
		for _, n := range nodes {
			if n.EffectiveRole() == fleetv1alpha1.NodeRoleControl {
				controls++
			}
		}
		if controls != 3 {
			return nil, fmt.Errorf("ha_3_node requires exactly 3 control nodes")
		}
		if req.BootstrapHost != nodes[0].Address {
			return nil, fmt.Errorf("bootstrapHost must match the first control node address")
		}
		nodeAddrs := make([]string, 0, len(nodes))
		for _, n := range nodes {
			if n.EffectiveRole() == fleetv1alpha1.NodeRoleControl {
				nodeAddrs = append(nodeAddrs, n.Address)
			}
		}
		if err := storage.ValidateControlPlaneVip(req.ControlPlaneVip, topology, nodeAddrs); err != nil {
			return nil, err
		}
	}
	if req.KubevirtBridge.Enabled && !req.Install.Kubevirt {
		return nil, fmt.Errorf("kubevirtBridge.enabled requires providers.install.kubevirt")
	}
	if req.Install.Kubevirt {
		if req.ClusterCapabilities == nil {
			req.ClusterCapabilities = map[string]bool{}
		}
		req.ClusterCapabilities["kvm"] = true
	}
	var site fleetv1alpha1.FleetSite
	if err := c.Get(ctx, client.ObjectKey{Namespace: cfg.Namespace, Name: req.SiteId}, &site); err != nil {
		if apierrors.IsNotFound(err) {
			if err2 := c.Get(ctx, client.ObjectKey{Namespace: cfg.Namespace, Name: req.ClusterName}, &site); err2 != nil || site.Spec.EffectiveSiteID(site.Name) != req.SiteId {
				return nil, fmt.Errorf("site not found")
			}
		} else {
			return nil, err
		}
	}
	site.Spec.NormalizeIngress()
	if _, err := getManagedCluster(ctx, c, req.ClusterName); err == nil {
		return nil, fmt.Errorf("cluster already exists in OCM")
	}
	var existing fleetv1alpha1.FleetClusterJoin
	if err := c.Get(ctx, client.ObjectKey{Namespace: cfg.Namespace, Name: req.ClusterName}, &existing); err == nil {
		return nil, fmt.Errorf("cluster join already exists")
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}
	ingressID := strings.TrimSpace(req.Edge.IngressId)
	if ingressID == "" {
		ingressID = site.Spec.AutoIngressID()
	}
	if ingressID != "" {
		bindings, err := existingIngressBindings(ctx, c, cfg, req.SiteId, req.ClusterName)
		if err != nil {
			return nil, err
		}
		if err := ingress.ValidateBinding(site.Spec, ingressID, req.ClusterName, bindings); err != nil {
			return nil, err
		}
	}
	minDisk := req.MinRootDiskGi
	if minDisk == 0 {
		minDisk = 40
	}
	fcj := fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.ClusterName,
			Namespace: cfg.Namespace,
		},
		Spec: fleetv1alpha1.FleetClusterJoinSpec{
			ClusterName:         req.ClusterName,
			SiteId:              req.SiteId,
			BootstrapHost:       req.BootstrapHost,
			Jumphost:            strings.TrimSpace(req.Jumphost),
			MinRootDiskGi:       minDisk,
			ClusterRegion:       req.ClusterRegion,
			ClusterLabels:       req.ClusterLabels,
			ClusterCapabilities: req.ClusterCapabilities,
			GPU:                 req.GPU,
			Edge: fleetv1alpha1.FleetClusterJoinEdgeSpec{
				IngressId: ingressID,
			},
			Topology:        topology,
			NetworkMode:     networkMode,
			RockyVersion:    cfg.RockyMajor(req.RockyVersion),
			HubLanApi:       req.HubLanApi,
			ControlPlaneVip: strings.TrimSpace(req.ControlPlaneVip),
			Nodes:           nodes,
		},
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			Phase:   fleetv1alpha1.JoinPhasePending,
			Message: "Awaiting bootstrap script",
		},
	}
	if req.Storage != nil {
		st := *req.Storage
		if st.NFSLocalEnabled() {
			if st.NFS == nil {
				st.NFS = &fleetv1alpha1.ClusterNFSSpec{}
			}
			if strings.TrimSpace(st.NFS.ExportBase) == "" {
				st.NFS.ExportBase = "/srv/fleet-nfs"
			}
			if strings.TrimSpace(st.NFS.StorageClass) == "" {
				st.NFS.StorageClass = "nfs-client"
			}
			if strings.TrimSpace(st.NFS.ClusterPath) == "" {
				st.NFS.ClusterPath = req.ClusterName
			}
			if strings.TrimSpace(st.NFS.Server) == "" {
				st.NFS.Server = strings.TrimSpace(req.BootstrapHost)
			}
			st.Rwx = true
			if !st.Rwo {
				st.Rwo = true
			}
		}
		stCopy := st
		fcj.Spec.Storage = &stCopy
		if err := storage.ValidateNFSSpec(&stCopy, &fcj); err != nil {
			return nil, err
		}
		if err := storage.ValidateLonghornSpec(&stCopy, topology); err != nil {
			return nil, err
		}
	}
	ports, err := providers.AllocateNodePorts(ctx, c, cfg.Namespace, req.Install)
	if err != nil {
		return nil, err
	}
	fcj.Spec.Providers = fleetv1alpha1.ClusterProvidersSpec{
		K8sNodePort:        ports.K8s,
		HelmNodePort:       ports.Helm,
		KubevirtSPNodeport: ports.Kubevirt,
		Install:            req.Install,
		KubevirtBridge:     req.KubevirtBridge,
	}
	if err := c.Create(ctx, &fcj); err != nil {
		return nil, err
	}
	return &fcj, nil
}

func getManagedCluster(ctx context.Context, c client.Client, name string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(managedClusterGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: name}, obj); err != nil {
		return nil, err
	}
	return obj, nil
}
