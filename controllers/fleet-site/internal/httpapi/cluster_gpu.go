package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/gpucatalog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	configureGpuHostAnnotation    = "fleet.umkim.com/configure-gpu-host"
	configureGpuHostJobAnnotation = "fleet.umkim.com/configure-gpu-host-job"
	configureGpuEnableAnnotation  = "fleet.umkim.com/configure-gpu-enable"
	configureGpuEnableJobAnnotation = "fleet.umkim.com/configure-gpu-enable-job"
)

type gpuHostPrepBody struct {
	Force bool `json:"force,omitempty"`
}

type clusterGpuStatusJSON struct {
	HostPhase        string `json:"hostPhase,omitempty"`
	HostJobName      string `json:"hostJobName,omitempty"`
	HostMessage      string `json:"hostMessage,omitempty"`
	RuntimePhase     string `json:"runtimePhase,omitempty"`
	RuntimeJobName   string `json:"runtimeJobName,omitempty"`
	RuntimeMessage   string `json:"runtimeMessage,omitempty"`
	PciID            string `json:"pciId,omitempty"`
	MarketingName    string `json:"marketingName,omitempty"`
	DriverBranch     string `json:"driverBranch,omitempty"`
	DriverLoaded     bool   `json:"driverLoaded,omitempty"`
	ClusterGpu       bool   `json:"clusterGpuCapability,omitempty"`
	GpuTotal         int    `json:"gpuTotal,omitempty"`
	GpuAvailable     int    `json:"gpuAvailable,omitempty"`
	RecommendedBranch string `json:"recommendedDriverBranch,omitempty"`
}

func (s *Server) handleClusterGpu(w http.ResponseWriter, r *http.Request, clusterName, rest string) {
	rest = strings.Trim(rest, "/")
	switch {
	case rest == "" && r.Method == http.MethodGet:
		s.handleGpuStatus(r.Context(), w, clusterName)
	case rest == "host-prep" && r.Method == http.MethodPost:
		s.handleGpuHostPrep(r.Context(), w, r, clusterName)
	case rest == "enable" && r.Method == http.MethodPost:
		s.handleGpuEnable(r.Context(), w, r, clusterName)
	case rest == "discover" && r.Method == http.MethodPost:
		s.handleDiscoverCapabilities(r.Context(), w, clusterName)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleGpuStatus(ctx context.Context, w http.ResponseWriter, clusterName string) {
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster join not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := clusterGpuStatusJSON{
		HostPhase:    fcj.Status.Gpu.HostPhase,
		HostJobName:  fcj.Status.Gpu.HostJobName,
		HostMessage:  fcj.Status.Gpu.HostMessage,
		RuntimePhase: fcj.Status.Gpu.RuntimePhase,
		RuntimeJobName: fcj.Status.Gpu.RuntimeJobName,
		RuntimeMessage: fcj.Status.Gpu.RuntimeMessage,
		PciID:        fcj.Status.Gpu.PciID,
		MarketingName: fcj.Status.Gpu.MarketingName,
		DriverBranch: fcj.Status.Gpu.DriverBranch,
		DriverLoaded: fcj.Status.Gpu.DriverLoaded,
	}
	if out.HostPhase == "" {
		out.HostPhase = fleetv1alpha1.GpuHostPhaseNotPrepared
	}
	if out.RuntimePhase == "" {
		out.RuntimePhase = fleetv1alpha1.GpuRuntimePhaseNotEnabled
	}
	mc, err := getManagedCluster(ctx, s.Client, clusterName)
	if err == nil && mc != nil {
		disc := discoveryFromMC(mc.GetAnnotations())
		if disc != nil {
			if disc.ClusterCapabilities != nil {
				out.ClusterGpu = disc.ClusterCapabilities["gpu"]
			}
			if disc.ClusterResources != nil {
				out.GpuTotal = disc.ClusterResources["gpu.total"]
				out.GpuAvailable = disc.ClusterResources["gpu.available"]
			}
			for _, n := range disc.Nodes {
				if strings.TrimSpace(n.NvidiaPciID) != "" {
					out.PciID = n.NvidiaPciID
					out.DriverLoaded = n.DriverLoaded
					if dev, ok := gpucatalog.LookupPCI(n.NvidiaPciID); ok {
						out.MarketingName = dev.MarketingName
						out.DriverBranch = dev.DriverBranch
						out.RecommendedBranch = dev.DriverBranch
					}
					break
				}
			}
		}
	}
	if out.RecommendedBranch == "" && out.DriverBranch != "" {
		out.RecommendedBranch = out.DriverBranch
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGpuHostPrep(ctx context.Context, w http.ResponseWriter, r *http.Request, clusterName string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req gpuHostPrepBody
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
	if fcj.Status.Gpu.HostPhase == fleetv1alpha1.GpuHostPhaseApplying {
		http.Error(w, "GPU host prep already running", http.StatusConflict)
		return
	}
	if fcj.Status.Phase == fleetv1alpha1.JoinPhaseReady && !req.Force {
		http.Error(w, "cluster is Ready; set force=true for post-bootstrap GPU host prep", http.StatusBadRequest)
		return
	}
	if fcj.Annotations != nil {
		if _, ok := fcj.Annotations[configureGpuHostAnnotation]; ok {
			if fcj.Status.Gpu.HostPhase != fleetv1alpha1.GpuHostPhaseFailed {
				http.Error(w, "GPU host prep already requested", http.StatusConflict)
				return
			}
			delete(fcj.Annotations, configureGpuHostAnnotation)
			delete(fcj.Annotations, configureGpuHostJobAnnotation)
		}
	}
	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	fcj.Annotations[configureGpuHostAnnotation] = time.Now().UTC().Format(time.RFC3339)
	if err := s.Client.Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fcj.Status.Gpu.HostPhase = fleetv1alpha1.GpuHostPhaseApplying
	fcj.Status.Gpu.HostMessage = "GPU host prep requested"
	fcj.Status.Message = fcj.Status.Gpu.HostMessage
	if err := s.Client.Status().Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"phase":   fleetv1alpha1.GpuHostPhaseApplying,
		"message": fcj.Status.Gpu.HostMessage,
	})
}

func (s *Server) handleGpuEnable(ctx context.Context, w http.ResponseWriter, r *http.Request, clusterName string) {
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
		http.Error(w, "cluster must be Ready", http.StatusBadRequest)
		return
	}
	if fcj.Status.Gpu.RuntimePhase == fleetv1alpha1.GpuRuntimePhaseApplying {
		http.Error(w, "GPU runtime enable already running", http.StatusConflict)
		return
	}
	if fcj.Status.Gpu.RuntimePhase == fleetv1alpha1.GpuRuntimePhaseReady {
		writeJSON(w, http.StatusAccepted, map[string]string{
			"phase":   fleetv1alpha1.GpuRuntimePhaseReady,
			"message": "GPU runtime already enabled",
		})
		return
	}
	if fcj.Annotations != nil {
		if _, ok := fcj.Annotations[configureGpuEnableAnnotation]; ok {
			if fcj.Status.Gpu.RuntimePhase != fleetv1alpha1.GpuRuntimePhaseFailed {
				http.Error(w, "GPU runtime enable already requested", http.StatusConflict)
				return
			}
			delete(fcj.Annotations, configureGpuEnableAnnotation)
			delete(fcj.Annotations, configureGpuEnableJobAnnotation)
		}
	}
	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	fcj.Annotations[configureGpuEnableAnnotation] = time.Now().UTC().Format(time.RFC3339)
	if err := s.Client.Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fcj.Status.Gpu.RuntimePhase = fleetv1alpha1.GpuRuntimePhaseApplying
	fcj.Status.Gpu.RuntimeMessage = "GPU runtime enable requested"
	fcj.Status.Message = fcj.Status.Gpu.RuntimeMessage
	if err := s.Client.Status().Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"phase":   fleetv1alpha1.GpuRuntimePhaseApplying,
		"message": fcj.Status.Gpu.RuntimeMessage,
	})
}
