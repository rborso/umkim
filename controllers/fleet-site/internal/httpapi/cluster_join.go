package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/bundle"
	"github.com/rborso/umkim/controllers/fleet-site/internal/clustercreate"
	"github.com/rborso/umkim/controllers/fleet-site/internal/providers"
	"github.com/rborso/umkim/controllers/fleet-site/internal/storage"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type clusterCreateBody struct {
	ClusterName         string            `json:"clusterName"`
	SiteId              string            `json:"siteId"`
	BootstrapHost       string            `json:"bootstrapHost"`
	Jumphost            string            `json:"jumphost,omitempty"`
	MinRootDiskGi       int               `json:"minRootDiskGi,omitempty"`
	ClusterRegion       string            `json:"clusterRegion,omitempty"`
	ClusterLabels       map[string]string `json:"clusterLabels,omitempty"`
	ClusterCapabilities map[string]bool   `json:"clusterCapabilities,omitempty"`
	GPU                 bool              `json:"gpu,omitempty"`
	Storage             *struct {
		LocalPath *struct {
			Enabled *bool `json:"enabled,omitempty"`
		} `json:"localPath,omitempty"`
		Backend       string `json:"backend,omitempty"`
		MinDataDiskGi int    `json:"minDataDiskGi,omitempty"`
		Rwo           bool   `json:"rwo,omitempty"`
		Rwx           bool   `json:"rwx,omitempty"`
		NFS           *struct {
			Enabled      bool   `json:"enabled,omitempty"`
			Server       string `json:"server,omitempty"`
			ExportBase   string `json:"exportBase,omitempty"`
			ClusterPath  string `json:"clusterPath,omitempty"`
			StorageClass string `json:"storageClass,omitempty"`
			DelegateHost string `json:"delegateHost,omitempty"`
			MountPath    string `json:"mountPath,omitempty"`
		} `json:"nfs,omitempty"`
		Longhorn *struct {
			Enabled      bool   `json:"enabled,omitempty"`
			Device       string `json:"device,omitempty"`
			StorageClass string `json:"storageClass,omitempty"`
		} `json:"longhorn,omitempty"`
	} `json:"storage,omitempty"`
	Edge            fleetv1alpha1.FleetClusterJoinEdgeSpec   `json:"edge,omitempty"`
	Topology        string                                   `json:"topology,omitempty"`
	NetworkMode     string                                   `json:"networkMode,omitempty"`
	RockyVersion    string                                   `json:"rockyVersion,omitempty"`
	HubLanApi       string                                   `json:"hubLanApi,omitempty"`
	ControlPlaneVip string                                   `json:"controlPlaneVip,omitempty"`
	Nodes           []fleetv1alpha1.FleetClusterJoinNodeSpec `json:"nodes,omitempty"`
	AutoProvision   bool                                     `json:"autoProvision,omitempty"`
	Providers       *struct {
		Install        fleetv1alpha1.ProviderInstallSpec `json:"install,omitempty"`
		KubevirtBridge fleetv1alpha1.KubevirtBridgeSpec  `json:"kubevirtBridge,omitempty"`
	} `json:"providers,omitempty"`
}

func (s *Server) getFleetClusterJoin(ctx context.Context, name string) (*fleetv1alpha1.FleetClusterJoin, error) {
	var fcj fleetv1alpha1.FleetClusterJoin
	err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: name}, &fcj)
	if err != nil {
		return nil, err
	}
	return &fcj, nil
}

// findFleetClusterJoin resolves by CR metadata.name or spec.clusterName (list API may use the latter).
func (s *Server) findFleetClusterJoin(ctx context.Context, clusterName string) (*fleetv1alpha1.FleetClusterJoin, error) {
	if fcj, err := s.getFleetClusterJoin(ctx, clusterName); err == nil {
		return fcj, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}
	var list fleetv1alpha1.FleetClusterJoinList
	if err := s.Client.List(ctx, &list, client.InNamespace(s.Cfg.Namespace)); err != nil {
		return nil, err
	}
	for i := range list.Items {
		if list.Items[i].Spec.EffectiveClusterName(list.Items[i].Name) == clusterName {
			return &list.Items[i], nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "fleet.umkim.com", Resource: "fleetclusterjoins"}, clusterName)
}

func (s *Server) listPendingClusterJoins(ctx context.Context) ([]fleetv1alpha1.FleetClusterJoin, error) {
	var list fleetv1alpha1.FleetClusterJoinList
	if err := s.Client.List(ctx, &list, client.InNamespace(s.Cfg.Namespace)); err != nil {
		return nil, err
	}
	out := make([]fleetv1alpha1.FleetClusterJoin, 0)
	for i := range list.Items {
		if list.Items[i].Status.Phase != fleetv1alpha1.JoinPhaseReady {
			out = append(out, list.Items[i])
		}
	}
	return out, nil
}

func clusterJoinProvisioningMode(fcj *fleetv1alpha1.FleetClusterJoin) string {
	if fcj.Spec.NetworkMode == fleetv1alpha1.NetworkModeLAN {
		return "hub_job"
	}
	return "bundle"
}

func clusterJoinUpstreamHost(fcj *fleetv1alpha1.FleetClusterJoin) string {
	if fcj.Status.WireguardAddress != "" {
		return strings.Split(fcj.Status.WireguardAddress, "/")[0]
	}
	return fcj.Spec.BootstrapHost
}

func (s *Server) fleetClusterJoinsByClusterName(ctx context.Context) map[string]*fleetv1alpha1.FleetClusterJoin {
	out := map[string]*fleetv1alpha1.FleetClusterJoin{}
	var list fleetv1alpha1.FleetClusterJoinList
	if err := s.Client.List(ctx, &list, client.InNamespace(s.Cfg.Namespace)); err != nil {
		return out
	}
	for i := range list.Items {
		name := list.Items[i].Spec.EffectiveClusterName(list.Items[i].Name)
		out[name] = &list.Items[i]
	}
	return out
}

// applyFleetClusterJoinSummary overlays FleetClusterJoin status onto an OCM-backed cluster summary.
func applyFleetClusterJoinSummary(cs *clusterSummary, fcj *fleetv1alpha1.FleetClusterJoin) {
	if fcj.Spec.Jumphost != "" {
		cs.Jumphost = fcj.Spec.Jumphost
	}
	if fcj.Status.Phase == "" {
		return
	}
	cs.ProvisioningPhase = fcj.Status.Phase
	cs.ProvisioningMode = clusterJoinProvisioningMode(fcj)
	cs.ManagedClusterAvailable = fcj.Status.ManagedClusterAvailable
	if cs.Site == "" {
		cs.Site = fcj.Spec.SiteId
	}
	if fcj.Status.Phase == fleetv1alpha1.JoinPhaseDeleting {
		cs.Available = false
		return
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		cs.Available = false
	}
	applyStorageToSummary(cs, fcj)
	applyProvidersToSummary(cs, fcj)
	cs.IngressAuthEnabled = fcj.Spec.IngressAuth.Enabled
	if fcj.Status.IngressAuth.Phase != "" || fcj.Status.IngressAuth.JobName != "" || fcj.Status.IngressAuth.Message != "" ||
		fcj.Status.IngressAuth.PortalHost != "" || fcj.Status.IngressAuth.LLDAPWebUIHost != "" || len(fcj.Status.IngressAuth.AccessDomains) > 0 ||
		fcj.Spec.IngressAuth.Directory != "" {
		cs.IngressAuthStatus = &clusterIngressAuthStatusJSON{
			Phase:          fcj.Status.IngressAuth.Phase,
			JobName:        fcj.Status.IngressAuth.JobName,
			Message:        fcj.Status.IngressAuth.Message,
			SecretsRef:     fcj.Status.IngressAuth.SecretsRef,
			PortalHost:     fcj.Status.IngressAuth.PortalHost,
			LLDAPWebUIHost: fcj.Status.IngressAuth.LLDAPWebUIHost,
			AccessDomains:  append([]string(nil), fcj.Status.IngressAuth.AccessDomains...),
			Directory:      fcj.Spec.IngressAuth.Directory,
		}
	}
	cs.AcmeEnabled = fcj.Spec.Acme.Enabled
	if fcj.Status.Acme.Phase != "" || fcj.Status.Acme.JobName != "" || fcj.Status.Acme.Message != "" {
		cs.AcmeStatus = &clusterAcmeStatusJSON{
			Phase:   fcj.Status.Acme.Phase,
			JobName: fcj.Status.Acme.JobName,
			Message: fcj.Status.Acme.Message,
		}
	}
	if fcj.Status.NodeOperation != nil {
		op := fcj.Status.NodeOperation
		cs.NodeOperation = &clusterNodeOperationJSON{
			ID:        op.ID,
			Type:      op.Type,
			Node:      op.Node,
			Phase:     op.Phase,
			JobName:   op.JobName,
			Message:   op.Message,
			Force:     op.Force,
			ActorKind: op.ActorKind,
			ActorName: op.ActorName,
		}
	}
}

func applyStorageToSummary(cs *clusterSummary, fcj *fleetv1alpha1.FleetClusterJoin) {
	cs.Storage = storageSummaryFromSpec(fcj.Spec.Storage)
	if fcj.Status.Storage.Phase != "" || fcj.Status.Storage.JobName != "" || fcj.Status.Storage.Message != "" {
		cs.StorageStatus = &clusterStorageStatusJSON{
			Phase:   fcj.Status.Storage.Phase,
			JobName: fcj.Status.Storage.JobName,
			Message: fcj.Status.Storage.Message,
		}
	}
}

func applyProvidersToSummary(cs *clusterSummary, fcj *fleetv1alpha1.FleetClusterJoin) {
	if len(fcj.Status.Providers) == 0 && !fcj.Spec.Providers.Install.Kubevirt {
		return
	}
	out := map[string]clusterProviderStatusJSON{}
	for kind, st := range fcj.Status.Providers {
		out[kind] = clusterProviderStatusJSON{
			Phase:        st.Phase,
			JobName:      st.JobName,
			Message:      st.Message,
			ProviderName: st.ProviderName,
		}
	}
	if _, ok := out[fleetv1alpha1.ProviderKindKubevirt]; !ok && fcj.Spec.Providers.Install.Kubevirt {
		out[fleetv1alpha1.ProviderKindKubevirt] = clusterProviderStatusJSON{
			Phase: fleetv1alpha1.ProviderPhaseNotInstalled,
		}
	}
	cs.Providers = out
}

func clusterSummaryFromJoin(fcj *fleetv1alpha1.FleetClusterJoin) clusterSummary {
	name := fcj.Spec.EffectiveClusterName(fcj.Name)
	cs := clusterSummary{
		Name:                    name,
		Site:                    fcj.Spec.SiteId,
		Available:               false,
		ProvisioningPhase:       fcj.Status.Phase,
		ManagedClusterAvailable: fcj.Status.ManagedClusterAvailable,
		Capabilities:            fcj.Spec.Capabilities(),
		Topology: map[string]string{
			"region": fcj.Spec.ClusterRegion, "provisioner": "k3s", "nest_depth": "0",
			"topology": fcj.Spec.Topology,
		},
		ProvisioningMode: clusterJoinProvisioningMode(fcj),
		IngressID:        fcj.Spec.Edge.IngressId,
		Jumphost:         fcj.Spec.Jumphost,
		UpstreamHost:     clusterJoinUpstreamHost(fcj),
		TraefikHTTPPort:  30080,
	}
	if fcj.Spec.Edge.IngressId != "" {
		cs.IngressID = fcj.Spec.Edge.IngressId
	}
	applyStorageToSummary(&cs, fcj)
	applyProvidersToSummary(&cs, fcj)
	return cs
}

func (s *Server) handleCreateCluster(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req clusterCreateBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ClusterName == "" || req.SiteId == "" || req.BootstrapHost == "" {
		http.Error(w, "clusterName, siteId, and bootstrapHost required", http.StatusBadRequest)
		return
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
		http.Error(w, "networkMode lan is deprecated; use wireguard overlay", http.StatusBadRequest)
		return
	}
	nodes, err := normalizeClusterCreateNodes(req.ClusterName, topology, req.BootstrapHost, req.Nodes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Nodes = nodes
	if topology == fleetv1alpha1.ClusterTopologySingleNode {
		if req.BootstrapHost != req.Nodes[0].Address {
			http.Error(w, "bootstrapHost must match the control node address", http.StatusBadRequest)
			return
		}
	}
	if topology == fleetv1alpha1.ClusterTopologyHA3Node {
		controls := 0
		for _, n := range req.Nodes {
			if n.EffectiveRole() == fleetv1alpha1.NodeRoleControl {
				controls++
			}
		}
		if controls != 3 {
			http.Error(w, "ha_3_node requires exactly 3 control nodes", http.StatusBadRequest)
			return
		}
		if req.BootstrapHost != req.Nodes[0].Address {
			http.Error(w, "bootstrapHost must match the first control node address", http.StatusBadRequest)
			return
		}
		nodeAddrs := make([]string, 0, len(req.Nodes))
		for _, n := range req.Nodes {
			if n.EffectiveRole() == fleetv1alpha1.NodeRoleControl {
				nodeAddrs = append(nodeAddrs, n.Address)
			}
		}
		if err := storage.ValidateControlPlaneVip(req.ControlPlaneVip, topology, nodeAddrs); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	install := fleetv1alpha1.ProviderInstallSpec{}
	var kubevirtBridge fleetv1alpha1.KubevirtBridgeSpec
	if req.Providers != nil {
		install = req.Providers.Install
		kubevirtBridge = req.Providers.KubevirtBridge
	}
	if kubevirtBridge.Enabled && !install.Kubevirt {
		http.Error(w, "kubevirtBridge.enabled requires providers.install.kubevirt", http.StatusBadRequest)
		return
	}
	if install.Kubevirt {
		if req.ClusterCapabilities == nil {
			req.ClusterCapabilities = map[string]bool{}
		}
		req.ClusterCapabilities["kvm"] = true
	}
	var site fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: req.SiteId}, &site); err != nil {
		if apierrors.IsNotFound(err) {
			if err2 := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: req.ClusterName}, &site); err2 != nil || site.Spec.EffectiveSiteID(site.Name) != req.SiteId {
				http.Error(w, "site not found", http.StatusBadRequest)
				return
			}
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if isManagementFleetSite(&site, site.Spec.EffectiveSiteID(site.Name), s.managementSiteID()) {
		http.Error(w, "cannot create clusters on management site", http.StatusBadRequest)
		return
	}
	if err := validateJumphostValue(req.Jumphost); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ingressID := strings.TrimSpace(req.Edge.IngressId)
	if ingressID == "" {
		ingressID = site.Spec.AutoIngressID()
	}
	if ingressID != "" {
		if err := s.validateClusterIngressBinding(ctx, req.SiteId, req.ClusterName, ingressID); err != nil {
			writeIngressConflict(w, err)
			return
		}
	}
	actor, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	_ = actor
	if _, err := getManagedCluster(ctx, s.Client, req.ClusterName); err == nil {
		http.Error(w, "cluster already exists in OCM", http.StatusConflict)
		return
	}
	if _, err := s.getFleetClusterJoin(ctx, req.ClusterName); err == nil {
		http.Error(w, "cluster join already exists", http.StatusConflict)
		return
	}
	minDisk := req.MinRootDiskGi
	if minDisk == 0 {
		minDisk = 40
	}
	fcj := fleetv1alpha1.FleetClusterJoin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.ClusterName,
			Namespace: s.Cfg.Namespace,
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
			RockyVersion:    s.Cfg.RockyMajor(req.RockyVersion),
			HubLanApi:       req.HubLanApi,
			ControlPlaneVip: strings.TrimSpace(req.ControlPlaneVip),
			Nodes:           req.Nodes,
			AutoProvision:   req.AutoProvision,
		},
		Status: fleetv1alpha1.FleetClusterJoinStatus{
			Phase:   fleetv1alpha1.JoinPhasePending,
			Message: "Awaiting bootstrap script",
		},
	}
	if req.Storage != nil {
		st := storageSpecFromCreateBody(req.Storage)
		if st != nil && st.NFSLocalEnabled() {
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
		fcj.Spec.Storage = st
		if err := storage.ValidateNFSSpec(st, &fcj); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := storage.ValidateLonghornSpec(st, topology); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	ports, err := providers.AllocateNodePorts(ctx, s.Client, s.Cfg.Namespace, install)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fcj.Spec.Providers = fleetv1alpha1.ClusterProvidersSpec{
		K8sNodePort:        ports.K8s,
		HelmNodePort:       ports.Helm,
		KubevirtSPNodeport: ports.Kubevirt,
		Install:            install,
		KubevirtBridge:     kubevirtBridge,
	}
	if err := s.Client.Create(ctx, &fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, clusterSummaryFromJoin(&fcj))
}

func (s *Server) handleClusterEnrollment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1alpha1/cluster-enrollments/")
	path = strings.Trim(path, "/")
	parts := strings.SplitN(path, "/", 2)
	clusterName := parts[0]
	if clusterName == "" {
		http.Error(w, "cluster name required", http.StatusBadRequest)
		return
	}
	if len(parts) == 2 && parts[1] == "progress" {
		s.handleClusterProgress(w, r, clusterName)
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	var body enrollRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	ctx := r.Context()
	fcj, err := s.getFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster join not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !clusterTokenMatches(ctx, s.Client, s.Cfg.Namespace, clusterName, token) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	secretPub := strings.TrimSpace(clusterSecretWgPublicKey(ctx, s.Client, s.Cfg.Namespace, clusterName))
	if body.WgPublicKey != "" {
		switch {
		case secretPub != "" && body.WgPublicKey == secretPub:
			fcj.Status.WireguardPublicKey = secretPub
		case fcj.Status.WireguardPublicKey != "" && body.WgPublicKey != fcj.Status.WireguardPublicKey:
			http.Error(w, "public key mismatch", http.StatusBadRequest)
			return
		case fcj.Status.WireguardPublicKey == "":
			fcj.Status.WireguardPublicKey = body.WgPublicKey
		}
	}
	now := metav1.Now()
	if fcj.Status.Phase == fleetv1alpha1.JoinPhaseEnrolled || fcj.Status.Phase == fleetv1alpha1.JoinPhaseValidated ||
		fcj.Status.Phase == fleetv1alpha1.JoinPhaseProvisioning || fcj.Status.Phase == fleetv1alpha1.JoinPhaseReady {
		writeJSON(w, http.StatusOK, map[string]string{"phase": fcj.Status.Phase, "message": "already enrolled"})
		return
	}
	fcj.Status.Phase = fleetv1alpha1.JoinPhaseEnrolled
	fcj.Status.Message = "Bootstrap script completed enrollment"
	fcj.Status.EnrolledAt = &now
	fcj.Status.EnrolledHostname = body.Hostname
	if err := s.Client.Status().Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"phase": fleetv1alpha1.JoinPhaseEnrolled})
}

type clusterProgressBody struct {
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

func (s *Server) handleClusterProgress(w http.ResponseWriter, r *http.Request, clusterName string) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	var body clusterProgressBody
	_ = json.NewDecoder(r.Body).Decode(&body)
	ctx := r.Context()
	fcj, err := s.getFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster join not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !clusterTokenMatches(ctx, s.Client, s.Cfg.Namespace, clusterName, token) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseEnrolled {
		writeJSON(w, http.StatusOK, map[string]string{"phase": fcj.Status.Phase, "message": "progress ignored for phase " + fcj.Status.Phase})
		return
	}
	switch body.Phase {
	case fleetv1alpha1.JoinPhaseValidated:
		fcj.Status.Phase = fleetv1alpha1.JoinPhaseValidated
		if body.Message != "" {
			fcj.Status.Message = body.Message
		} else {
			fcj.Status.Message = "Local bundle complete; use Console Provision for hub finalize"
		}
		if fcj.Spec.Storage != nil && fcj.Spec.Storage.EffectiveBackend() == fleetv1alpha1.StorageBackendNFSExternal {
			fcj.Status.Storage.Phase = fleetv1alpha1.StoragePhaseApplied
			fcj.Status.Storage.Message = "NFS provisioner installed during local provision"
		}
		if fcj.Spec.Storage != nil && fcj.Spec.Storage.NFSLocalEnabled() {
			fcj.Status.Storage.Phase = fleetv1alpha1.StoragePhaseApplied
			fcj.Status.Storage.Message = "Local NFS server and provisioner installed during local provision"
		}
		if fcj.Spec.Storage != nil && fcj.Spec.Storage.LonghornEnabled() {
			fcj.Status.Storage.Phase = fleetv1alpha1.StoragePhaseApplied
			fcj.Status.Storage.Message = "Longhorn installed during local provision"
		}
	case fleetv1alpha1.JoinPhaseFailed:
		fcj.Status.Phase = fleetv1alpha1.JoinPhaseFailed
		fcj.Status.Message = body.Message
		if fcj.Status.Message == "" {
			fcj.Status.Message = "Local bundle failed"
		}
	default:
		http.Error(w, "unsupported progress phase", http.StatusBadRequest)
		return
	}
	if err := s.Client.Status().Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"phase": fcj.Status.Phase})
}

func (s *Server) serveClusterBootstrapBundle(ctx context.Context, w http.ResponseWriter, name string) {
	var secret corev1.Secret
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: clusterBootstrapSecretName(name)}, &secret); err != nil {
		http.Error(w, "bundle not issued", http.StatusNotFound)
		return
	}
	archiveBytes := secret.Data[bundle.BootstrapArchiveKey]
	if len(archiveBytes) == 0 {
		http.Error(w, "bundle not available (use bootstrap-script or re-issue)", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename=bootstrap-"+name+".tar.gz")
	_, _ = w.Write(archiveBytes)
}

func clusterBootstrapSecretName(clusterName string) string {
	return "fleet-cluster-" + clusterName + "-bootstrap"
}

func clusterTokenMatches(ctx context.Context, c client.Client, ns, clusterName, token string) bool {
	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: clusterBootstrapSecretName(clusterName)}, &secret); err != nil {
		return false
	}
	stored := strings.TrimSpace(string(secret.Data["enrollment-token"]))
	return stored != "" && stored == token
}

func (s *Server) serveClusterBootstrapScript(ctx context.Context, w http.ResponseWriter, name string) {
	var secret corev1.Secret
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: clusterBootstrapSecretName(name)}, &secret); err != nil {
		http.Error(w, "script not issued", http.StatusNotFound)
		return
	}
	script := string(secret.Data["bootstrap.sh"])
	if script == "" {
		script = secret.StringData["bootstrap.sh"]
	}
	w.Header().Set("Content-Type", "text/x-shellscript")
	w.Header().Set("Content-Disposition", "attachment; filename=bootstrap-"+name+".sh")
	_, _ = w.Write([]byte(script))
}

func (s *Server) triggerClusterAction(ctx context.Context, w http.ResponseWriter, name, action string) {
	fcj, err := s.getFleetClusterJoin(ctx, name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if action == "provision" && fcj.Status.Phase == fleetv1alpha1.JoinPhaseReady {
		clusterName := fcj.Spec.EffectiveClusterName(fcj.Name)
		if mc, err := getManagedCluster(ctx, s.Client, clusterName); err == nil && mcConditionAvailable(mc) {
			http.Error(w, "cluster is already provisioned", http.StatusConflict)
			return
		}
	}
	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	fcj.Annotations["fleet.umkim.com/"+action] = "true"
	if err := s.Client.Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"action": action})
}

func clusterSecretWgPublicKey(ctx context.Context, c client.Client, ns, clusterName string) string {
	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: clusterBootstrapSecretName(clusterName)}, &secret); err != nil {
		return ""
	}
	return string(secret.Data["wg-public-key"])
}

// normalizeClusterCreateNodes delegates to the shared create validator so the
// API path and the runbook/flow path enforce the same node rules (including the
// optional vm block).
func normalizeClusterCreateNodes(clusterName, topology, bootstrapHost string, nodes []fleetv1alpha1.FleetClusterJoinNodeSpec) ([]fleetv1alpha1.FleetClusterJoinNodeSpec, error) {
	return clustercreate.NormalizeNodes(clusterName, topology, bootstrapHost, nodes)
}
