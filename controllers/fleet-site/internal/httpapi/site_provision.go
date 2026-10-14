package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors/yaidap"
	"github.com/rborso/umkim/controllers/fleet-site/internal/provision"
	"github.com/rborso/umkim/controllers/fleet-site/internal/sitehosts"
	"github.com/rborso/umkim/controllers/fleet-site/internal/siteprovision"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	maxProvisionHistory    = 20
	siteProvisionInvCMFmt  = "fleet-site-%s-provision-inventory"
	siteProvisionJobPrefix = "fleet-site-prov-"
)

type siteProvisionBindingView struct {
	Provider           string                             `json:"provider"`
	Jumphost           string                             `json:"jumphost,omitempty"`
	SSHSecretRef       string                             `json:"sshSecretRef,omitempty"`
	InventoryPath      string                             `json:"inventoryPath,omitempty"`
	YaidaPRoot         string                             `json:"yaidapRoot,omitempty"`
	YaidapRepo         string                             `json:"yaidapRepo,omitempty"`
	YaidapRef          string                             `json:"yaidapRef,omitempty"`
	YaidapGitSecretRef string                             `json:"yaidapGitSecretRef,omitempty"`
	DefaultExtraVars   map[string]string                  `json:"defaultExtraVars,omitempty"`
	Status             *fleetv1alpha1.SiteProvisionStatus `json:"status,omitempty"`
}

type siteProvisionBindingBody struct {
	Provider           string            `json:"provider"`
	Jumphost           string            `json:"jumphost,omitempty"`
	SSHSecretRef       string            `json:"sshSecretRef,omitempty"`
	InventoryPath      string            `json:"inventoryPath,omitempty"`
	YaidaPRoot         string            `json:"yaidapRoot,omitempty"`
	YaidapRepo         string            `json:"yaidapRepo,omitempty"`
	YaidapRef          string            `json:"yaidapRef,omitempty"`
	YaidapGitSecretRef string            `json:"yaidapGitSecretRef,omitempty"`
	DefaultExtraVars   map[string]string `json:"defaultExtraVars,omitempty"`
}

type siteProvisionActionBody struct {
	Action   string                              `json:"action"`
	Subject  string                              `json:"subject"`
	Confirm  bool                                `json:"confirm"`
	Provider string                              `json:"provider,omitempty"`
	Params   map[string]interface{}              `json:"params,omitempty"`
	Host     *fleetv1alpha1.ProvisionHandoffHost `json:"host,omitempty"`
}

func (s *Server) handleSiteProvisionRoutes(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName, rest string) {
	if strings.HasPrefix(rest, "provision/inventory") {
		s.handleSiteInventoryRoutes(ctx, w, r, siteName, rest)
		return
	}
	switch {
	case rest == "provision":
		switch r.Method {
		case http.MethodGet:
			s.getSiteProvisionBinding(ctx, w, siteName)
		case http.MethodPut, http.MethodPatch:
			s.putSiteProvisionBinding(ctx, w, r, siteName)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case rest == "provision/yaidap/sync":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var fs fleetv1alpha1.FleetSite
		if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		s.syncSiteYaidapRepo(ctx, w, r, siteName, &fs)
	case rest == "provision/actions":
		switch r.Method {
		case http.MethodGet:
			s.listSiteProvisionActions(ctx, w, siteName)
		case http.MethodPost:
			s.startSiteProvisionAction(ctx, w, r, siteName)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case strings.HasPrefix(rest, "provision/actions/"):
		id, wantLogs, ok := provisionActionSubpath(strings.TrimPrefix(rest, "provision/actions/"))
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if wantLogs {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			s.handleSiteProvisionActionLogs(ctx, w, siteName, id)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.getSiteProvisionAction(ctx, w, siteName, id)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) getSiteProvisionBinding(ctx context.Context, w http.ResponseWriter, siteName string) {
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.refreshProvisionJobIfNeeded(ctx, &fs)
	writeJSON(w, http.StatusOK, siteProvisionBindingFromSite(&fs))
}

func (s *Server) putSiteProvisionBinding(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName string) {
	_, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req siteProvisionBindingBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = fleetv1alpha1.ProvisionProviderNoop
	}
	if err := provision.ValidateProvider(provider); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if jh := strings.TrimSpace(req.Jumphost); jh != "" {
		if err := validateJumphostValue(jh); err != nil {
			http.Error(w, "invalid jumphost: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	spec := &fleetv1alpha1.SiteProvisionSpec{
		Provider:           provider,
		Jumphost:           strings.TrimSpace(req.Jumphost),
		SSHSecretRef:       strings.TrimSpace(req.SSHSecretRef),
		InventoryPath:      strings.TrimSpace(req.InventoryPath),
		YaidaPRoot:         strings.TrimSpace(req.YaidaPRoot),
		YaidapRepo:         strings.TrimSpace(req.YaidapRepo),
		YaidapRef:          strings.TrimSpace(req.YaidapRef),
		YaidapGitSecretRef: strings.TrimSpace(req.YaidapGitSecretRef),
		DefaultExtraVars:   req.DefaultExtraVars,
	}

	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if r.Method == http.MethodPatch && fs.Spec.Provision != nil {
		mergeProvisionSpec(fs.Spec.Provision, spec)
		spec = fs.Spec.Provision
		spec.Provider = provider
	}
	if provider == fleetv1alpha1.ProvisionProviderYaidap {
		if err := provision.ValidateYaidapBinding(spec); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	fs.Spec.Provision = spec
	if err := s.Client.Update(ctx, &fs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, siteProvisionBindingFromSite(&fs))
}

func mergeProvisionSpec(dst, src *fleetv1alpha1.SiteProvisionSpec) {
	if src.Jumphost != "" {
		dst.Jumphost = src.Jumphost
	}
	if src.SSHSecretRef != "" {
		dst.SSHSecretRef = src.SSHSecretRef
	}
	if src.InventoryPath != "" {
		dst.InventoryPath = src.InventoryPath
	}
	if src.YaidaPRoot != "" {
		dst.YaidaPRoot = src.YaidaPRoot
	}
	if src.YaidapRepo != "" {
		dst.YaidapRepo = src.YaidapRepo
	}
	if src.YaidapRef != "" {
		dst.YaidapRef = src.YaidapRef
	}
	if src.YaidapGitSecretRef != "" {
		dst.YaidapGitSecretRef = src.YaidapGitSecretRef
	}
	if src.DefaultExtraVars != nil {
		dst.DefaultExtraVars = src.DefaultExtraVars
	}
}

func siteProvisionBindingFromSite(fs *fleetv1alpha1.FleetSite) siteProvisionBindingView {
	v := siteProvisionBindingView{
		Provider: fs.Spec.EffectiveProvisionProvider(),
		Status:   fs.Status.Provision,
	}
	if fs.Spec.Provision != nil {
		v.Jumphost = fs.Spec.Provision.Jumphost
		v.SSHSecretRef = fs.Spec.Provision.SSHSecretRef
		v.InventoryPath = fs.Spec.Provision.InventoryPath
		v.YaidaPRoot = fs.Spec.Provision.YaidaPRoot
		v.YaidapRepo = fs.Spec.Provision.YaidapRepo
		v.YaidapRef = fs.Spec.Provision.YaidapRef
		v.YaidapGitSecretRef = fs.Spec.Provision.YaidapGitSecretRef
		v.DefaultExtraVars = fs.Spec.Provision.DefaultExtraVars
	}
	return v
}

func (s *Server) listSiteProvisionActions(ctx context.Context, w http.ResponseWriter, siteName string) {
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.refreshProvisionJobIfNeeded(ctx, &fs)
	writeJSON(w, http.StatusOK, siteProvisionHistoryList{Items: provisionHistoryItems(&fs)})
}

func (s *Server) getSiteProvisionAction(ctx context.Context, w http.ResponseWriter, siteName, id string) {
	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.refreshProvisionJobIfNeeded(ctx, &fs)
	rec, ok := findProvisionAction(&fs, id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) startSiteProvisionAction(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req siteProvisionActionBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.runSiteProvisionAction(ctx, w, r, siteName, req)
}

func (s *Server) runSiteProvisionAction(ctx context.Context, w http.ResponseWriter, r *http.Request, siteName string, req siteProvisionActionBody) {
	actor, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	action := strings.TrimSpace(req.Action)
	subject := strings.TrimSpace(req.Subject)
	if err := provision.ValidateAction(action); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if subject == "" {
		http.Error(w, "subject is required", http.StatusBadRequest)
		return
	}
	if provision.RequiresConfirm(action) && !req.Confirm {
		http.Error(w, "confirm=true is required for "+action, http.StatusBadRequest)
		return
	}
	invHost, guestFound, guestErr := yaidap.LookupInventoryHost(ctx, s.Client, s.Cfg.Namespace, siteName, subject)
	if guestErr != nil {
		http.Error(w, guestErr.Error(), http.StatusInternalServerError)
		return
	}
	if !guestFound {
		http.Error(w, fmt.Sprintf("host %q not found in inventory", subject), http.StatusBadRequest)
		return
	}
	hostKind := strings.TrimSpace(invHost.Kind)
	if hostKind == "" {
		if invHost.External {
			hostKind = fleetv1alpha1.SiteHostKindBareMetal
		} else {
			hostKind = fleetv1alpha1.SiteHostKindVM
		}
	}
	if err := yaidap.ValidateActionForKind(action, hostKind); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if action == fleetv1alpha1.ProvisionActionPlanVM {
		if !guestFound || invHost.External {
			http.Error(w, "plan_vm requires an existing provisioned VM guest in guests.yml", http.StatusBadRequest)
			return
		}
	}
	if action == fleetv1alpha1.ProvisionActionDiscover {
		if err := yaidap.ValidateDiscoverAction(ctx, s.Client, s.Cfg.Namespace, siteName, subject, invHost, guestFound); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if action == fleetv1alpha1.ProvisionActionDiscoverPXE {
		if err := yaidap.ValidateDiscoverPXEAction(ctx, s.Client, s.Cfg.Namespace, siteName, subject); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if action == fleetv1alpha1.ProvisionActionDestroy {
		if err := yaidap.ValidateDestroyAction(ctx, s.Client, s.Cfg.Namespace, siteName, subject, invHost, guestFound); err != nil {
			if errors.Is(err, yaidap.ErrDestroyClusterConflict) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if req.Host == nil && guestFound {
		if addr := strings.TrimSpace(invHost.AnsibleHost); addr != "" {
			req.Host = &fleetv1alpha1.ProvisionHandoffHost{
				Hostname:    subject,
				AnsibleHost: addr,
				AnsibleUser: "linux",
				OSFamily:    "rocky",
				SiteID:      siteName,
			}
		}
	}

	var fs fleetv1alpha1.FleetSite
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.refreshProvisionJobIfNeeded(ctx, &fs)
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.Namespace, Name: siteName}, &fs); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if provisionActionBusy(&fs) {
		http.Error(w, "site provision action already running", http.StatusConflict)
		return
	}

	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = fs.Spec.EffectiveProvisionProvider()
	}
	if err := provision.ValidateProvider(provider); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	now := metav1.NewTime(time.Now().UTC())
	rec := fleetv1alpha1.SiteProvisionActionRecord{
		ID:        newProvisionActionID(),
		Action:    action,
		Subject:   subject,
		Provider:  provider,
		Phase:     fleetv1alpha1.ProvisionPhasePending,
		Confirm:   req.Confirm,
		ActorKind: actor.Kind,
		ActorName: actor.Name,
		StartedAt: &now,
	}

	switch provider {
	case fleetv1alpha1.ProvisionProviderNoop:
		host := resolveNoopHost(req, subject, fs.Spec.EffectiveSiteID(fs.Name))
		if host == nil || strings.TrimSpace(host.AnsibleHost) == "" {
			http.Error(w, "noop provider requires host.ansible_host (or params.host)", http.StatusBadRequest)
			return
		}
		end := metav1.NewTime(time.Now().UTC())
		rec.Phase = fleetv1alpha1.ProvisionPhaseSucceeded
		rec.Message = "noop: host fragment accepted"
		rec.Host = host
		rec.EndedAt = &end
		if err := s.commitProvisionAction(ctx, &fs, rec, true); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, rec)

	case fleetv1alpha1.ProvisionProviderYaidap:
		if err := provision.ValidateYaidapBinding(fs.Spec.Provision); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := yaidap.EnsureReadyForProvision(ctx, s.Client, s.Cfg.Namespace, siteName, connectors.FormatActor(actor.Kind, actor.Name)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if guestFound && provision.ActionSyncsYaidapInventory(action) {
			kind := strings.TrimSpace(invHost.Kind)
			if kind == "" && !invHost.External {
				kind = fleetv1alpha1.SiteHostKindVM
			}
			vmLoc := strings.TrimSpace(invHost.VmLocation)
			if vmLoc == "" {
				vmLoc = strings.TrimSpace(invHost.Status.VmLocation)
			}
			if err := sitehosts.ValidateVmLocation(kind, vmLoc); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		jobName, err := s.startYaidapProvisionJob(ctx, &fs, &rec, req, guestFound)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rec.Phase = fleetv1alpha1.ProvisionPhaseRunning
		rec.Message = "YAIDAP connector job started"
		rec.JobName = jobName
		if err := s.commitProvisionAction(ctx, &fs, rec, false); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusAccepted, rec)

	default:
		http.Error(w, "unknown provider", http.StatusBadRequest)
	}
}

func resolveNoopHost(req siteProvisionActionBody, subject, siteID string) *fleetv1alpha1.ProvisionHandoffHost {
	if req.Host != nil {
		h := *req.Host
		if h.Hostname == "" {
			h.Hostname = subject
		}
		if h.SiteID == "" {
			h.SiteID = siteID
		}
		return &h
	}
	if req.Params != nil {
		if raw, ok := req.Params["host"]; ok {
			b, err := json.Marshal(raw)
			if err == nil {
				var h fleetv1alpha1.ProvisionHandoffHost
				if json.Unmarshal(b, &h) == nil {
					if h.Hostname == "" {
						h.Hostname = subject
					}
					if h.SiteID == "" {
						h.SiteID = siteID
					}
					return &h
				}
			}
		}
		if ah, ok := req.Params["ansible_host"].(string); ok && strings.TrimSpace(ah) != "" {
			return &fleetv1alpha1.ProvisionHandoffHost{
				Hostname:    subject,
				AnsibleHost: strings.TrimSpace(ah),
				AnsibleUser: stringParam(req.Params, "ansible_user"),
				OSFamily:    stringParam(req.Params, "osFamily"),
				SiteID:      siteID,
			}
		}
	}
	return nil
}

func stringParam(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func (s *Server) startYaidapProvisionJob(ctx context.Context, fs *fleetv1alpha1.FleetSite, rec *fleetv1alpha1.SiteProvisionActionRecord, req siteProvisionActionBody, guestFound bool) (string, error) {
	return siteprovision.StartYaidapAction(ctx, s.Client, s.Cfg, fs, rec, siteprovision.StartYaidapActionOptions{
		Params:               req.Params,
		GuestFound:           guestFound,
		HubFleetSSHPublicKey: s.hubFleetSSHPublicKey(ctx),
		ActorKind:            rec.ActorKind,
		ActorName:            rec.ActorName,
	})
}
func jumphostInventoryINI(jumphost, defaultUser string) string {
	user, host, port := parseJumphost(jumphost, defaultUser)
	var b strings.Builder
	b.WriteString("[jumphost]\n")
	b.WriteString("jumphost ansible_host=")
	b.WriteString(host)
	b.WriteString(" ansible_user=")
	b.WriteString(user)
	if port != "" && port != "22" {
		b.WriteString(" ansible_port=")
		b.WriteString(port)
	}
	b.WriteString("\n")
	return b.String()
}

func parseJumphost(raw, defaultUser string) (user, host, port string) {
	user = defaultUser
	if user == "" {
		user = "linux"
	}
	port = "22"
	raw = strings.TrimSpace(raw)
	if at := strings.LastIndex(raw, "@"); at >= 0 {
		user = raw[:at]
		raw = raw[at+1:]
	}
	if i := strings.LastIndex(raw, ":"); i >= 0 {
		// Distinguish host:port from IPv6 — only split if suffix is numeric-ish.
		maybePort := raw[i+1:]
		if maybePort != "" && !strings.Contains(maybePort, ":") {
			port = maybePort
			raw = raw[:i]
		}
	}
	host = raw
	return user, host, port
}

func (s *Server) commitProvisionAction(ctx context.Context, fs *fleetv1alpha1.FleetSite, rec fleetv1alpha1.SiteProvisionActionRecord, finished bool) error {
	return siteprovision.CommitAction(ctx, s.Client, fs, rec, finished)
}

func provisionActionBusy(fs *fleetv1alpha1.FleetSite) bool {
	return siteprovision.ActionBusy(fs)
}

func findProvisionAction(fs *fleetv1alpha1.FleetSite, id string) (fleetv1alpha1.SiteProvisionActionRecord, bool) {
	if fs.Status.Provision == nil {
		return fleetv1alpha1.SiteProvisionActionRecord{}, false
	}
	if fs.Status.Provision.Current != nil && fs.Status.Provision.Current.ID == id {
		return *fs.Status.Provision.Current, true
	}
	for _, h := range fs.Status.Provision.History {
		if h.ID == id {
			return h, true
		}
	}
	return fleetv1alpha1.SiteProvisionActionRecord{}, false
}

func (s *Server) refreshProvisionJobIfNeeded(ctx context.Context, fs *fleetv1alpha1.FleetSite) {
	cs, _ := s.localClientset()
	_ = siteprovision.RefreshRunningJob(ctx, s.Client, s.Cfg.Namespace, fs, cs, &s.Cfg)
}

func newProvisionActionID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
