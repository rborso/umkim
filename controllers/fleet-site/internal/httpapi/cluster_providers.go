package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type clusterProviderRow struct {
	Kind         string `json:"kind"`
	DisplayName  string `json:"displayName"`
	ProviderName string `json:"providerName"`
	NodePort     int    `json:"nodePort,omitempty"`
	Phase        string `json:"phase"`
	Message      string `json:"message,omitempty"`
}

func providerAnnotation(kind, action string) string {
	return fmt.Sprintf("fleet.umkim.com/provider-%s-%s", kind, action)
}

func (s *Server) handleClusterProviders(w http.ResponseWriter, r *http.Request, clusterName, rest string) {
	if rest == "" && r.Method == http.MethodGet {
		s.handleListClusterProviders(r.Context(), w, clusterName)
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	kind, action := parts[0], parts[1]
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "install", "delete":
		s.triggerClusterProviderAction(r.Context(), w, clusterName, kind, action)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleListClusterProviders(ctx context.Context, w http.ResponseWriter, clusterName string) {
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := fcj.Spec.EffectiveClusterName(fcj.Name)
	rows := []clusterProviderRow{
		providerRowFromFCJ(fcj, name, fleetv1alpha1.ProviderKindKubevirt, "Virtualization (KubeVirt)", fcj.Spec.Providers.KubevirtSPNodeport),
		providerRowFromFCJ(fcj, name, fleetv1alpha1.ProviderKindK8s, "Container workloads", fcj.Spec.Providers.K8sNodePort),
		providerRowFromFCJ(fcj, name, fleetv1alpha1.ProviderKindHelm, "Helm releases", fcj.Spec.Providers.HelmNodePort),
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"providers": rows})
}

func providerRowFromFCJ(fcj *fleetv1alpha1.FleetClusterJoin, clusterName, kind, display string, nodePort int) clusterProviderRow {
	providerName := providerDisplayName(fcj, clusterName, kind)
	phase := fleetv1alpha1.ProviderPhaseNotInstalled
	message := ""
	if fcj.Status.Providers != nil {
		if st, ok := fcj.Status.Providers[kind]; ok {
			if st.Phase != "" {
				phase = st.Phase
			}
			message = st.Message
			if st.ProviderName != "" {
				providerName = st.ProviderName
			}
		}
	}
	return clusterProviderRow{
		Kind:         kind,
		DisplayName:  display,
		ProviderName: providerName,
		NodePort:     nodePort,
		Phase:        phase,
		Message:      message,
	}
}

func providerDisplayName(fcj *fleetv1alpha1.FleetClusterJoin, clusterName, kind string) string {
	switch kind {
	case fleetv1alpha1.ProviderKindHelm:
		return fcj.Spec.HelmProviderName(clusterName)
	case fleetv1alpha1.ProviderKindKubevirt:
		return fcj.Spec.KubevirtProviderName(clusterName)
	default:
		return fcj.Spec.K8sProviderName(clusterName)
	}
}

func isSupportedProviderKind(kind string) bool {
	switch kind {
	case fleetv1alpha1.ProviderKindK8s, fleetv1alpha1.ProviderKindHelm,
		fleetv1alpha1.ProviderKindKubevirt:
		return true
	default:
		return false
	}
}

func (s *Server) triggerClusterProviderAction(ctx context.Context, w http.ResponseWriter, clusterName, kind, action string) {
	if !isSupportedProviderKind(kind) {
		http.Error(w, "unsupported provider kind", http.StatusBadRequest)
		return
	}
	fcj, err := s.findFleetClusterJoin(ctx, clusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "cluster not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fcj.Status.Phase != fleetv1alpha1.JoinPhaseReady {
		http.Error(w, "cluster must be Ready", http.StatusConflict)
		return
	}
	if action == "install" {
		if kind == fleetv1alpha1.ProviderKindKubevirt {
			if !clusterHasKVMCapability(ctx, s, clusterName) {
				http.Error(w, "cluster lacks kvm capability (run Discover capabilities)", http.StatusConflict)
				return
			}
		}
	}
	if fcj.Annotations == nil {
		fcj.Annotations = map[string]string{}
	}
	ann := providerAnnotation(kind, action)
	if _, ok := fcj.Annotations[ann]; ok {
		http.Error(w, "provider action already requested", http.StatusConflict)
		return
	}
	for k := range fcj.Annotations {
		if strings.HasPrefix(k, "fleet.umkim.com/provider-") {
			http.Error(w, "another provider action is pending", http.StatusConflict)
			return
		}
	}
	if fcj.Status.Providers != nil {
		for _, st := range fcj.Status.Providers {
			if st.JobName != "" && (st.Phase == fleetv1alpha1.ProviderPhaseInstalling || st.Phase == fleetv1alpha1.ProviderPhaseDeleting) {
				http.Error(w, "provider job already running", http.StatusConflict)
				return
			}
		}
	}
	fcj.Annotations[ann] = "true"
	fcj.Spec.Providers.Install.SetIntent(kind, action == "install")
	if err := s.Client.Update(ctx, fcj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"kind": kind, "action": action})
}
