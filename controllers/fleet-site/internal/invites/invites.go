// Package invites implements admin-minted, single-use directory invitations.
//
// An invite is a random one-time token plus the tenant groups a redeemed user
// should join. The token is stored only as a SHA-256 hash in a Kubernetes
// Secret; the raw value is shown once at creation.
package invites

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultTTL is the default invite lifetime.
const DefaultTTL = 72 * time.Hour

// Invite is one single-use directory invitation.
type Invite struct {
	ID              string   `json:"id"`
	TokenHash       string   `json:"tokenHash"`
	Groups          []string `json:"groups,omitempty"`
	DisplayNameHint string   `json:"displayNameHint,omitempty"`
	EmailHint       string   `json:"emailHint,omitempty"`
	Note            string   `json:"note,omitempty"`
	CreatedBy       string   `json:"createdBy,omitempty"`
	CreatedAt       string   `json:"createdAt"`
	ExpiresAt       string   `json:"expiresAt"`
	Used            bool     `json:"used,omitempty"`
	UsedAt          string   `json:"usedAt,omitempty"`
	RedeemedBy      string   `json:"redeemedBy,omitempty"`
}

// NewToken returns a fresh 256-bit URL-safe token.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken returns the hex SHA-256 of a raw token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewID returns a fresh 128-bit invite identifier.
func NewID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// NormalizeGroups trims, de-duplicates and sorts group names.
func NormalizeGroups(in []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, g := range in {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// Expired reports whether the invite is past its expiry.
func (i Invite) Expired(now time.Time) bool {
	exp, err := time.Parse(time.RFC3339, i.ExpiresAt)
	if err != nil {
		return true
	}
	return now.After(exp)
}

// Load reads the invite Secret. A missing Secret yields an empty map.
func Load(ctx context.Context, c client.Client, ns, name string) (map[string]Invite, error) {
	var sec corev1.Secret
	err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &sec)
	if apierrors.IsNotFound(err) {
		return map[string]Invite{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]Invite{}
	for k, v := range sec.Data {
		var inv Invite
		if err := json.Unmarshal(v, &inv); err != nil {
			continue
		}
		if inv.ID == "" {
			inv.ID = k
		}
		out[k] = inv
	}
	return out, nil
}

// Save writes the full invite map to the Secret (create or update).
func Save(ctx context.Context, c client.Client, ns, name string, invites map[string]Invite) error {
	data := map[string][]byte{}
	for k, inv := range invites {
		b, err := json.Marshal(inv)
		if err != nil {
			return err
		}
		data[k] = b
	}
	var sec corev1.Secret
	err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &sec)
	if apierrors.IsNotFound(err) {
		return c.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      name,
				Labels:    map[string]string{"app.kubernetes.io/name": "fleet-invites"},
			},
			Type: corev1.SecretTypeOpaque,
			Data: data,
		})
	}
	if err != nil {
		return err
	}
	sec.Data = data
	return c.Update(ctx, &sec)
}

// Create adds one invite, returning an error if the id already exists.
func Create(ctx context.Context, c client.Client, ns, name string, inv Invite) error {
	all, err := Load(ctx, c, ns, name)
	if err != nil {
		return err
	}
	all[inv.ID] = inv
	return Save(ctx, c, ns, name, all)
}

// Get returns one invite by id.
func Get(ctx context.Context, c client.Client, ns, name, id string) (Invite, bool, error) {
	all, err := Load(ctx, c, ns, name)
	if err != nil {
		return Invite{}, false, err
	}
	inv, ok := all[id]
	return inv, ok, nil
}

// Delete removes one invite.
func Delete(ctx context.Context, c client.Client, ns, name, id string) error {
	all, err := Load(ctx, c, ns, name)
	if err != nil {
		return err
	}
	if _, ok := all[id]; !ok {
		return nil
	}
	delete(all, id)
	return Save(ctx, c, ns, name, all)
}

// FindByToken returns the invite whose token hash matches, if any.
func FindByToken(ctx context.Context, c client.Client, ns, name, token string) (Invite, bool, error) {
	all, err := Load(ctx, c, ns, name)
	if err != nil {
		return Invite{}, false, err
	}
	want := HashToken(token)
	for _, inv := range all {
		if inv.TokenHash == want {
			return inv, true, nil
		}
	}
	return Invite{}, false, nil
}

// MarkUsed records a successful redemption.
func MarkUsed(ctx context.Context, c client.Client, ns, name, id, username string) error {
	all, err := Load(ctx, c, ns, name)
	if err != nil {
		return err
	}
	inv, ok := all[id]
	if !ok {
		return nil
	}
	inv.Used = true
	inv.UsedAt = time.Now().UTC().Format(time.RFC3339)
	inv.RedeemedBy = username
	all[id] = inv
	return Save(ctx, c, ns, name, all)
}
