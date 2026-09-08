package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	configureKubevirtAnnotation    = "fleet.umkim.com/configure-kubevirt"
	configureKubevirtJobAnnotation = "fleet.umkim.com/configure-kubevirt-job"
)

type enableKubevirtBody struct {
	LanBridge  bool   `json:"lanBridge,omitempty"`
	BridgeName string `json:"bridgeName,omitempty"`
	SlaveNic   string `json:"slaveNic,omitempty"`
	KeepIP     *bool  `json:"keepIp,omitempty"`
	Force      bool   `json:"force,omitempty"`
}

func (s *Server) handleClusterPlatform(w http.ResponseWriter, r *http.Request, clusterName, rest string) {
	rest = strings.Trim(rest, "/")
	switch {
	case rest == "kubevirt" && r.Method == http.MethodPost:
		s.handleEnableKubevirt(r.Context(), w, r, clusterName)
	case rest == "kubevirt" && r.Method == http.MethodGet:
		s.handleKubevirtStatus(r.Context(), w, clusterName)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleKubevirtStatus(ctx context.Context, w http.ResponseWriter, clusterName string) {
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster join not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	st := fcj.Status.Providers[fleetv1alpha1.ProviderKindKubevirt]
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"phase":      st.Phase,
		"jobName":    st.JobName,
		"message":    st.Message,
		"install":    fcj.Spec.Providers.Install.Kubevirt,
		"lanBridge":  fcj.Spec.WantsKubevirtBridge(),
		"bridgeName": fcj.Spec.Providers.KubevirtBridge.EffectiveBridgeName(),
	})
}

func (s *Server) handleEnableKubevirt(ctx context.Context, w http.ResponseWriter, r *http.Request, clusterName string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req enableKubevirtBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster join not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		http.Error(w, "cluster must be Ready", http.StatusConflict)
		return
	}
	edge, _ := loadEdgeClusters(ctx, s.Client, s.Cfg.EdgeClustersCMNamespace, s.Cfg.EdgeClustersCMName)
	if s.clusterReadOnly(clusterName, edge) {
		http.Error(w, "cluster is read-only", http.StatusForbidden)
		return
	}
	st := fcj.Status.Providers[fleetv1alpha1.ProviderKindKubevirt]
	if st.Phase == fleetv1alpha1.ProviderPhaseInstalling {
		http.Error(w, "KubeVirt install already running", http.StatusConflict)
		return
	}
	if fcj.Status.Storage.Phase == fleetv1alpha1.StoragePhaseApplying ||
		fcj.Status.Storage.Phase == fleetv1alpha1.StoragePhasePending {
		http.Error(w, "storage configure already running", http.StatusConflict)
		return
	}
	if fcj.Status.NodeOperation != nil &&
		(fcj.Status.NodeOperation.Phase == fleetv1alpha1.NodeOpPhasePending ||
			fcj.Status.NodeOperation.Phase == fleetv1alpha1.NodeOpPhaseRunning) {
		http.Error(w, "cluster node operation already running", http.StatusConflict)
		return
	}
	if fcj.Annotations != nil {
		if _, ok := fcj.Annotations[configureKubevirtAnnotation]; ok {
			if st.Phase != fleetv1alpha1.ProviderPhaseFailed {
				http.Error(w, "KubeVirt install already requested", http.StatusConflict)
				return
			}
			delete(fcj.Annotations, configureKubevirtAnnotation)
			delete(fcj.Annotations, configureKubevirtJobAnnotation)
		}
	}

	// Allow day-2 LAN Multus enable without --force when KubeVirt is already Ready
	// but the Multus host-bridge / NAD intent was not set yet.
	lanUpgrade := req.LanBridge && !fcj.Spec.WantsKubevirtBridge()
	if st.Phase == fleetv1alpha1.ProviderPhaseReady && !req.Force && !lanUpgrade {
		writeJSON(w, http.StatusAccepted, map[string]string{
			"phase":   fleetv1alpha1.ProviderPhaseReady,
			"message": "KubeVirt already installed",
		})
		return
	}

	fcj.Spec.Providers.Install.Kubevirt = true
	if req.LanBridge {
		keep := true
		if req.KeepIP != nil {
			keep = *req.KeepIP
		}
		bridgeName := strings.TrimSpace(req.BridgeName)
		if bridgeName == "" {
			bridgeName = "n0p0_mgmt_nw"
		}
		fcj.Spec.Providers.KubevirtBridge = fleetv1alpha1.KubevirtBridgeSpec{
			Enabled:    true,
			BridgeName: bridgeName,
			SlaveNic:   strings.TrimSpace(req.SlaveNic),
			KeepIP:     &keep,
		}
	} else if !fcj.Spec.Providers.KubevirtBridge.Enabled {
		fcj.Spec.Providers.KubevirtBridge = fleetv1alpha1.KubevirtBridgeSpec{}
	}

	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	fcj.Annotations[configureKubevirtAnnotation] = time.Now().UTC().Format(time.RFC3339)
	if err := s.Client.Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fcj.Status.Providers == nil {
		fcj.Status.Providers = map[string]fleetv1alpha1.ClusterProviderStatus{}
	}
	msg := "KubeVirt platform install requested"
	if lanUpgrade {
		msg = "LAN Multus (host bridge + NAD) enable requested"
	} else if req.LanBridge {
		msg = "KubeVirt platform install with LAN Multus requested"
	}
	fcj.Status.Providers[fleetv1alpha1.ProviderKindKubevirt] = fleetv1alpha1.ClusterProviderStatus{
		Phase:        fleetv1alpha1.ProviderPhaseInstalling,
		Message:      msg,
		ProviderName: fcj.Spec.KubevirtProviderName(clusterName),
	}
	fcj.Status.Message = msg
	if err := s.Client.Status().Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"phase":     fleetv1alpha1.ProviderPhaseInstalling,
		"lanBridge": fcj.Spec.WantsKubevirtBridge(),
		"message":   msg,
	})
}
