package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/provision"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitehosts"
	"k8s.io/apimachinery/pkg/types"
)

type siteHostBody struct {
	Name          string                         `json:"name"`
	Kind          string                         `json:"kind"`
	DesiredState  string                         `json:"desiredState,omitempty"`
	AnsibleHost   string                         `json:"ansible_host,omitempty"`
	AnsibleUser   string                         `json:"ansible_user,omitempty"`
	OSFamily      string                         `json:"osFamily,omitempty"`
	VmLocation    string                         `json:"vm_location,omitempty"`
	Flavor        map[string]string              `json:"flavor,omitempty"`
	Labels        map[string]string              `json:"labels,omitempty"`
	ClusterIntent *fleetv1alpha1.SiteHostClusterIntent `json:"clusterIntent,omitempty"`
	ProvisionNow  bool                           `json:"provisionNow,omitempty"`
}

type siteHostProvisionBody struct {
	Action  string `json:"action,omitempty"`
	Confirm bool   `json:"confirm"`
}

func (b siteHostBody) toRecord(nameOverride string) fleetv1alpha1.SiteHostRecord {
	name := strings.TrimSpace(b.Name)
	if nameOverride != "" {
		name = nameOverride
	}
	return fleetv1alpha1.SiteHostRecord{
		Name:          name,
		Kind:          strings.TrimSpace(b.Kind),
		DesiredState:  strings.TrimSpace(b.DesiredState),
		AnsibleHost:   strings.TrimSpace(b.AnsibleHost),
		AnsibleUser:   strings.TrimSpace(b.AnsibleUser),
		OSFamily:      strings.TrimSpace(b.OSFamily),
		VmLocation:    strings.TrimSpace(b.VmLocation),
		Flavor:        b.Flavor,
		Labels:        b.Labels,
		ClusterIntent: b.ClusterIntent,
		Status:        fleetv1alpha1.SiteHostStatus{Phase: fleetv1alpha1.SiteHostPhaseDeclared},
	}
}

func (s *Server) handleSiteHostRoutes(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, rest string) {
	if s.siteIsManagement(ctx, siteName) {
		http.Error(w, "management site has no host inventory", http.StatusForbidden)
		return
	}
	if !s.siteExists(ctx, siteName) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch {
	case rest == "hosts":
		switch r.Method {
		case http.MethodGet:
			s.listSiteHosts(ctx, w, siteName)
		case http.MethodPost:
			s.createSiteHost(ctx, w, r, siteName)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case strings.HasPrefix(rest, "hosts/"):
		sub := strings.TrimPrefix(rest, "hosts/")
		sub = strings.Trim(sub, "/")
		parts := strings.SplitN(sub, "/", 2)
		hostName := parts[0]
		if hostName == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		hostSub := ""
		if len(parts) == 2 {
			hostSub = parts[1]
		}
		switch hostSub {
		case "":
			switch r.Method {
			case http.MethodGet:
				s.getSiteHost(ctx, w, siteName, hostName)
			case http.MethodPatch, http.MethodPut:
				s.patchSiteHost(ctx, w, r, siteName, hostName)
			case http.MethodDelete:
				s.deleteSiteHost(ctx, w, r, siteName, hostName)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		case "provision":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			s.provisionSiteHost(ctx, w, r, siteName, hostName)
		case "cluster":
			s.handleSiteHostCluster(ctx, w, r, siteName, hostName)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) siteExists(ctx context.Context, siteName string) bool {
	var fs fleetv1alpha1.FleetSite
	err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs)
	return err == nil
}

func (s *Server) listSiteHosts(ctx context.Context, w http.ResponseWriter, siteName string) {
	hosts, err := sitehosts.List(ctx, s.Client, s.Cfg.Namespace, siteName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if hosts == nil {
		hosts = []fleetv1alpha1.SiteHostRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": hosts})
}

func (s *Server) getSiteHost(ctx context.Context, w http.ResponseWriter, siteName, hostName string) {
	rec, ok, err := sitehosts.Get(ctx, s.Client, s.Cfg.Namespace, siteName, hostName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) createSiteHost(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName string) {
	_, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req siteHostBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rec := req.toRecord("")
	if err := sitehosts.ValidateRecord(rec, false); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, exists, _ := sitehosts.Get(ctx, s.Client, s.Cfg.Namespace, siteName, rec.Name); exists {
		http.Error(w, "host already exists", http.StatusConflict)
		return
	}
	if err := sitehosts.Put(ctx, s.Client, s.Cfg.Namespace, siteName, rec); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if req.ProvisionNow {
		actionBody := siteProvisionActionBody{
			Action:  defaultProvisionActionForHost(rec),
			Subject: rec.Name,
			Confirm: rec.Kind == fleetv1alpha1.SiteHostKindBareMetal || rec.Status.Phase == fleetv1alpha1.SiteHostPhaseReady,
		}
		if rec.AnsibleHost != "" {
			actionBody.Host = hostRecordToHandoff(rec, siteName)
		}
		s.runSiteProvisionAction(ctx, w, r, siteName, actionBody)
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) patchSiteHost(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, hostName string) {
	_, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req siteHostBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	existing, found, err := sitehosts.Get(ctx, s.Client, s.Cfg.Namespace, siteName, hostName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	merged := mergeSiteHostRecord(existing, req)
	if err := sitehosts.ValidateRecord(merged, false); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := sitehosts.Put(ctx, s.Client, s.Cfg.Namespace, siteName, merged); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, merged)
}

func mergeSiteHostRecord(existing fleetv1alpha1.SiteHostRecord, patch siteHostBody) fleetv1alpha1.SiteHostRecord {
	out := existing
	if k := strings.TrimSpace(patch.Kind); k != "" {
		out.Kind = k
	}
	if d := strings.TrimSpace(patch.DesiredState); d != "" {
		out.DesiredState = d
	}
	if v := strings.TrimSpace(patch.AnsibleHost); v != "" {
		out.AnsibleHost = v
	}
	if v := strings.TrimSpace(patch.AnsibleUser); v != "" {
		out.AnsibleUser = v
	}
	if v := strings.TrimSpace(patch.OSFamily); v != "" {
		out.OSFamily = v
	}
	if v := strings.TrimSpace(patch.VmLocation); v != "" {
		out.VmLocation = v
	}
	if patch.Flavor != nil {
		out.Flavor = patch.Flavor
	}
	if patch.Labels != nil {
		out.Labels = patch.Labels
	}
	if patch.ClusterIntent != nil {
		out.ClusterIntent = patch.ClusterIntent
	}
	return out
}

func (s *Server) deleteSiteHost(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, hostName string) {
	_, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	if err := sitehosts.Delete(ctx, s.Client, s.Cfg.Namespace, siteName, hostName); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) provisionSiteHost(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, hostName string) {
	rec, found, err := sitehosts.Get(ctx, s.Client, s.Cfg.Namespace, siteName, hostName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req siteHostProvisionBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	action := strings.TrimSpace(req.Action)
	if action == "" {
		action = defaultProvisionActionForHost(rec)
	}
	if err := provision.ValidateAction(action); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if provision.RequiresConfirm(action) && !req.Confirm {
		http.Error(w, "confirm=true is required for "+action, http.StatusBadRequest)
		return
	}
	actionBody := siteProvisionActionBody{
		Action:  action,
		Subject: hostName,
		Confirm: req.Confirm,
	}
	if rec.AnsibleHost != "" {
		actionBody.Host = hostRecordToHandoff(rec, siteName)
	}
	s.runSiteProvisionAction(ctx, w, r, siteName, actionBody)
}

func defaultProvisionActionForHost(rec fleetv1alpha1.SiteHostRecord) string {
	switch rec.Kind {
	case fleetv1alpha1.SiteHostKindBareMetal:
		return "install_bm"
	default:
		return "create_vm"
	}
}

func hostRecordToHandoff(rec fleetv1alpha1.SiteHostRecord, siteName string) *fleetv1alpha1.ProvisionHandoffHost {
	user := rec.AnsibleUser
	if user == "" {
		user = "linux"
	}
	osFamily := rec.OSFamily
	if osFamily == "" {
		osFamily = "rocky"
	}
	return &fleetv1alpha1.ProvisionHandoffHost{
		Hostname:    rec.Name,
		AnsibleHost: rec.AnsibleHost,
		AnsibleUser: user,
		OSFamily:    osFamily,
		SiteID:      siteName,
	}
}
