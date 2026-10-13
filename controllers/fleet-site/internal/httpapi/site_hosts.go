package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/provision"
	"k8s.io/apimachinery/pkg/types"
)

type siteHostProvisionBody struct {
	Action  string `json:"action,omitempty"`
	Confirm bool   `json:"confirm"`
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
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.listSiteHosts(ctx, w, siteName)
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
			case http.MethodDelete:
				s.removeSiteHost(ctx, w, r, siteName, hostName)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		case "provision":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			s.provisionSiteHost(ctx, w, r, siteName, hostName)
		case "verify":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			s.verifySiteHost(ctx, w, r, siteName, hostName)
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
	_ = yaidap.MigrateLegacyStore(ctx, s.Client, s.Cfg.Namespace, siteName, "system/migrate")
	views, err := yaidap.ListInventoryHosts(ctx, s.Client, s.Cfg.Namespace, siteName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]interface{}, 0, len(views))
	for _, v := range views {
		rec := inventoryViewsToHostRecords([]yaidap.InventoryHostView{v})[0]
		row := map[string]interface{}{
			"name":          rec.Name,
			"kind":          rec.Kind,
			"ansible_host":  rec.AnsibleHost,
			"vm_location":   rec.VmLocation,
			"status":        rec.Status,
			"external":      v.External,
			"clusters":      v.Clusters,
		}
		items = append(items, row)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i]["name"].(string) < items[j]["name"].(string)
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func inventoryViewsToHostRecords(views []yaidap.InventoryHostView) []fleetv1alpha1.SiteHostRecord {
	out := make([]fleetv1alpha1.SiteHostRecord, 0, len(views))
	for _, v := range views {
		rec := fleetv1alpha1.SiteHostRecord{
			Name:        v.Name,
			Kind:        v.Kind,
			AnsibleHost: v.AnsibleHost,
			VmLocation:  v.VmLocation,
			Status: fleetv1alpha1.SiteHostStatus{
				Phase:       v.Status.Phase,
				Message:     v.Status.Message,
				AnsibleHost: v.AnsibleHost,
				Cluster:     v.Status.Cluster,
			},
		}
		if rec.Kind == "" {
			if v.External {
				rec.Kind = fleetv1alpha1.SiteHostKindBareMetal
			} else {
				rec.Kind = fleetv1alpha1.SiteHostKindVM
			}
		}
		out = append(out, rec)
	}
	return out
}

func (s *Server) getSiteHost(ctx context.Context, w http.ResponseWriter, siteName, hostName string) {
	views, err := yaidap.ListInventoryHosts(ctx, s.Client, s.Cfg.Namespace, siteName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, v := range views {
		if v.Name == hostName {
			items := inventoryViewsToHostRecords([]yaidap.InventoryHostView{v})
			writeJSON(w, http.StatusOK, items[0])
			return
		}
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func (s *Server) provisionSiteHost(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, hostName string) {
	exists, err := yaidap.GuestExists(ctx, s.Client, s.Cfg.Namespace, siteName, hostName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "guest not found in inventory", http.StatusNotFound)
		return
	}
	invHost, hostFound, hostErr := yaidap.LookupInventoryHost(ctx, s.Client, s.Cfg.Namespace, siteName, hostName)
	if hostErr != nil {
		http.Error(w, hostErr.Error(), http.StatusInternalServerError)
		return
	}
	if !hostFound {
		http.Error(w, "host not found in hub inventory store; PUT host_vars or run site inventory sync", http.StatusBadRequest)
		return
	}
	addr, _ := yaidap.GuestAddress(ctx, s.Client, s.Cfg.Namespace, siteName, hostName)
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
	hostKind := strings.TrimSpace(invHost.Kind)
	if hostKind == "" {
		if invHost.External {
			hostKind = fleetv1alpha1.SiteHostKindBareMetal
		} else {
			hostKind = fleetv1alpha1.SiteHostKindVM
		}
	}
	if action == "" {
		if hostKind != fleetv1alpha1.SiteHostKindVM {
			http.Error(w, "action is required for host kind "+hostKind, http.StatusBadRequest)
			return
		}
		action = fleetv1alpha1.ProvisionActionCreateVM
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
	if addr != "" {
		actionBody.Host = &fleetv1alpha1.ProvisionHandoffHost{
			Hostname:    hostName,
			AnsibleHost: addr,
			AnsibleUser: "linux",
			OSFamily:    "rocky",
			SiteID:      siteName,
		}
	}
	s.runSiteProvisionAction(ctx, w, r, siteName, actionBody)
}
