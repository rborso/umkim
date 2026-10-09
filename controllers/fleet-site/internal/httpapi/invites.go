package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-site/internal/invites"
	"github.com/rborso/umkim/controllers/fleet-site/internal/jobs"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	reservedInviteUsers = "admin,dir-admin,svc-bind"
	maxInviteTTLHours   = 24 * 30
)

var inviteUserRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

type inviteCreateBody struct {
	Groups          []string `json:"groups"`
	TTLHours        int      `json:"ttlHours"`
	DisplayNameHint string   `json:"displayNameHint"`
	EmailHint       string   `json:"emailHint"`
	Note            string   `json:"note"`
}

type inviteRedeemBody struct {
	Token       string `json:"token"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

type inviteView struct {
	ID              string   `json:"id"`
	Groups          []string `json:"groups,omitempty"`
	DisplayNameHint string   `json:"displayNameHint,omitempty"`
	EmailHint       string   `json:"emailHint,omitempty"`
	Note            string   `json:"note,omitempty"`
	CreatedBy       string   `json:"createdBy,omitempty"`
	CreatedAt       string   `json:"createdAt"`
	ExpiresAt       string   `json:"expiresAt"`
	Used            bool     `json:"used"`
	UsedAt          string   `json:"usedAt,omitempty"`
	RedeemedBy      string   `json:"redeemedBy,omitempty"`
	Expired         bool     `json:"expired"`
}

func inviteToView(inv invites.Invite, now time.Time) inviteView {
	return inviteView{
		ID:              inv.ID,
		Groups:          inv.Groups,
		DisplayNameHint: inv.DisplayNameHint,
		EmailHint:       inv.EmailHint,
		Note:            inv.Note,
		CreatedBy:       inv.CreatedBy,
		CreatedAt:       inv.CreatedAt,
		ExpiresAt:       inv.ExpiresAt,
		Used:            inv.Used,
		UsedAt:          inv.UsedAt,
		RedeemedBy:      inv.RedeemedBy,
		Expired:         inv.Expired(now),
	}
}

func (s *Server) handleInvites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listInvites(w, r)
	case http.MethodPost:
		s.createInvite(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleInvitesSub(w http.ResponseWriter, r *http.Request, sub string) {
	if sub == "redeem" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.redeemInvite(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.deleteInvite(w, r, sub)
}

func (s *Server) listInvites(w http.ResponseWriter, r *http.Request) {
	if !s.requireIdentity(w, r) {
		return
	}
	all, err := invites.Load(r.Context(), s.reader(), s.Cfg.Namespace, s.Cfg.InvitesSecretName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	views := make([]inviteView, 0, len(all))
	for _, inv := range all {
		views = append(views, inviteToView(inv, now))
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": views})
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	if !a.isPlatformAdmin() {
		http.Error(w, "forbidden: platform admin required", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req inviteCreateBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	groups := invites.NormalizeGroups(req.Groups)
	if len(groups) == 0 {
		http.Error(w, "at least one tenant group is required", http.StatusBadRequest)
		return
	}
	ttl := req.TTLHours
	if ttl <= 0 {
		ttl = int(invites.DefaultTTL.Hours())
	}
	if ttl > maxInviteTTLHours {
		http.Error(w, "ttlHours is too large", http.StatusBadRequest)
		return
	}
	id, err := invites.NewID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	token, err := invites.NewToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	inv := invites.Invite{
		ID:              id,
		TokenHash:       invites.HashToken(token),
		Groups:          groups,
		DisplayNameHint: strings.TrimSpace(req.DisplayNameHint),
		EmailHint:       strings.TrimSpace(req.EmailHint),
		Note:            strings.TrimSpace(req.Note),
		CreatedBy:       a.Name,
		CreatedAt:       now.Format(time.RFC3339),
		ExpiresAt:       now.Add(time.Duration(ttl) * time.Hour).Format(time.RFC3339),
	}
	if err := invites.Create(r.Context(), s.Client, s.Cfg.Namespace, s.Cfg.InvitesSecretName, inv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":        id,
		"token":     token,
		"path":      "/fleet/invite?token=" + token,
		"links":     s.inviteSignupLinks(r.Context(), token),
		"groups":    groups,
		"expiresAt": inv.ExpiresAt,
	})
}

// inviteSignupLink is a reachable sign-up URL for an invite.
type inviteSignupLink struct {
	Cluster string `json:"cluster"`
	URL     string `json:"url"`
}

// inviteSignupLinks returns publicly reachable sign-up pages for an invite:
// one per cluster that uses the shared hub directory (its Authelia portal host
// serves /invite), plus the hub console when a public console URL is configured.
func (s *Server) inviteSignupLinks(ctx context.Context, token string) []inviteSignupLink {
	links := []inviteSignupLink{}
	if base := strings.TrimRight(strings.TrimSpace(s.Cfg.ConsolePublicURL), "/"); base != "" {
		links = append(links, inviteSignupLink{Cluster: "hub", URL: base + "/fleet/invite?token=" + token})
	}
	var joins fleetv1alpha1.FleetClusterJoinList
	if err := s.reader().List(ctx, &joins, client.InNamespace(s.Cfg.Namespace)); err != nil {
		return links
	}
	for i := range joins.Items {
		fcj := &joins.Items[i]
		if !fcj.Spec.IngressAuth.Enabled || fcj.Spec.IngressAuth.Directory != "hub" {
			continue
		}
		host := strings.TrimSpace(fcj.Status.IngressAuth.PortalHost)
		if host == "" {
			continue
		}
		links = append(links, inviteSignupLink{
			Cluster: fcj.Spec.EffectiveClusterName(fcj.Name),
			URL:     "https://" + host + "/invite?token=" + token,
		})
	}
	return links
}

func (s *Server) deleteInvite(w http.ResponseWriter, r *http.Request, id string) {
	a, ok := s.requireMutatingActor(w, r)
	if !ok {
		return
	}
	if !a.isPlatformAdmin() {
		http.Error(w, "forbidden: platform admin required", http.StatusForbidden)
		return
	}
	if err := invites.Delete(r.Context(), s.Client, s.Cfg.Namespace, s.Cfg.InvitesSecretName, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) redeemInvite(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req inviteRedeemBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	token := strings.TrimSpace(req.Token)
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if token == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}
	if !inviteUserRe.MatchString(username) {
		http.Error(w, "username must be a lowercase DNS label (letters, digits, hyphen)", http.StatusBadRequest)
		return
	}
	for _, reserved := range strings.Split(reservedInviteUsers, ",") {
		if username == reserved {
			http.Error(w, "username is reserved", http.StatusBadRequest)
			return
		}
	}
	if len(req.Password) < 8 {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	inv, found, err := invites.FindByToken(r.Context(), s.reader(), s.Cfg.Namespace, s.Cfg.InvitesSecretName, token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "invalid invite token", http.StatusNotFound)
		return
	}
	now := time.Now().UTC()
	if inv.Used {
		http.Error(w, "invite has already been used", http.StatusConflict)
		return
	}
	if inv.Expired(now) {
		http.Error(w, "invite has expired", http.StatusGone)
		return
	}

	if err := s.provisionInvitedUser(r.Context(), inv, username, req.Password, req.DisplayName, req.Email); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := invites.MarkUsed(r.Context(), s.Client, s.Cfg.Namespace, s.Cfg.InvitesSecretName, inv.ID, username); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "username": username, "groups": inv.Groups})
}

func (s *Server) provisionInvitedUser(ctx context.Context, inv invites.Invite, username, password, displayName, email string) error {
	adminUser, adminPass, err := s.lldapServiceAccount(ctx)
	if err != nil {
		return err
	}
	lldap := invites.NewLLDAP(s.Cfg.LldapURL)
	token, err := lldap.Login(ctx, adminUser, adminPass)
	if err != nil {
		return err
	}
	exists, err := lldap.UserExists(ctx, token, username)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("directory user %q already exists", username)
	}
	if err := lldap.CreateUser(ctx, token, username, displayName, email); err != nil {
		return err
	}
	for _, g := range inv.Groups {
		gid, ok, err := lldap.GroupID(ctx, token, g)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("directory group %q does not exist", g)
		}
		if err := lldap.AddUserToGroup(ctx, token, username, gid); err != nil {
			return err
		}
	}
	return s.setInvitedUserPassword(ctx, username, password)
}

func (s *Server) lldapServiceAccount(ctx context.Context) (string, string, error) {
	var sec corev1.Secret
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Cfg.ConsoleNamespace, Name: s.Cfg.LldapServiceAccountsSecret}, &sec); err != nil {
		return "", "", fmt.Errorf("read LLDAP service accounts: %w", err)
	}
	user := strings.TrimSpace(string(sec.Data["admin-username"]))
	pass := string(sec.Data["admin-password"])
	if user == "" || pass == "" {
		return "", "", fmt.Errorf("LLDAP service account secret is incomplete")
	}
	return user, pass, nil
}

func (s *Server) setInvitedUserPassword(ctx context.Context, username, password string) error {
	ns := s.Cfg.Namespace
	suffix, err := invites.NewID()
	if err != nil {
		return err
	}
	suffix = suffix[:8]
	pwSecretName := "fleet-invite-pw-" + suffix
	pwSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      pwSecretName,
			Labels:    map[string]string{"app": "fleet-invite"},
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{"password": password},
	}
	if err := s.Client.Create(ctx, pwSecret); err != nil {
		return err
	}
	defer func() { _ = s.Client.Delete(context.Background(), pwSecret) }()

	jobName := "fleet-invite-pw-" + suffix
	job := jobs.InvitePasswordJob(ns, jobName, s.Cfg.LldapImage, s.Cfg.LldapURL, username, s.Cfg.LldapServiceAccountsSecret, pwSecretName)
	if err := s.Client.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	defer func() {
		_ = s.Client.Delete(context.Background(), job, client.PropagationPolicy(metav1.DeletePropagationBackground))
	}()

	deadline := time.Now().Add(150 * time.Second)
	seen := false
	for {
		var cur batchv1.Job
		if err := s.reader().Get(ctx, types.NamespacedName{Namespace: ns, Name: jobName}, &cur); err != nil {
			if apierrors.IsNotFound(err) && seen {
				// Job reaped after we observed it running; treat as complete.
				return nil
			}
			if apierrors.IsNotFound(err) {
				// Not visible yet (direct read lag); keep polling.
				if time.Now().After(deadline) {
					return fmt.Errorf("password job %s never appeared", jobName)
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(2 * time.Second):
				}
				continue
			}
			return err
		}
		seen = true
		for _, c := range cur.Status.Conditions {
			if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
				return nil
			}
			if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
				return fmt.Errorf("password job failed: %s", c.Message)
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("password job did not finish in time")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
