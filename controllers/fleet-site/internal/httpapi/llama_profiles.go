package httpapi

import (
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/llamaprofiles"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type llamaProfilesListResponse struct {
	Profiles []llamaprofiles.ProfileMeta `json:"profiles"`
}

type llamaProfileValuesResponse struct {
	ID         string `json:"id"`
	ValuesYAML string `json:"valuesYaml"`
}

func (s *Server) handleLlamaCppProfiles(w http.ResponseWriter, r *http.Request, subPath string) {
	subPath = strings.TrimPrefix(subPath, "llama-cpp-helm/profiles")
	subPath = strings.Trim(subPath, "/")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if subPath == "" {
		_ = s.ensureLlamaProfileSeed(r.Context())
		var list fleetv1alpha1.FleetLlamaProfileList
		if err := s.Client.List(r.Context(), &list, client.InNamespace(s.profileNS())); err == nil && len(list.Items) > 0 {
			sortByName(list.Items, func(p fleetv1alpha1.FleetLlamaProfile) string { return p.Name })
			writeJSON(w, http.StatusOK, llamaProfilesListResponse{Profiles: llamaprofiles.ListProfileMetasFromCRs(list.Items)})
			return
		}
		writeJSON(w, http.StatusOK, llamaProfilesListResponse{Profiles: llamaprofiles.List()})
		return
	}
	if p, err := llamaprofiles.ResolveProfile(r.Context(), s.Client, s.Cfg, subPath); err == nil {
		yaml, merr := llamaprofiles.MergeForProfile(p, s.Cfg.LlamaCppImageRepository())
		if merr != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": merr.Error()})
			return
		}
		writeJSON(w, http.StatusOK, llamaProfileValuesResponse{ID: subPath, ValuesYAML: yaml})
		return
	}
	if !llamaprofiles.ValidProfileID(subPath) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	yaml, err := llamaprofiles.MergeProfileYAML(subPath, s.Cfg.LlamaCppImageRepository())
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, llamaProfileValuesResponse{ID: subPath, ValuesYAML: yaml})
}
