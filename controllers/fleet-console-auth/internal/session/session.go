package session

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Sub        string   `json:"sub"`
	Username   string   `json:"username"`
	Groups     []string `json:"groups,omitempty"`
	AuthMethod string   `json:"authMethod"`
	Kind       string   `json:"kind"`
	jwt.RegisteredClaims
}

type Manager struct {
	secret     []byte
	ttl        time.Duration
	cookieName string
	secure     bool
}

func NewManager(secret string, ttl time.Duration, cookieName string, secure bool) *Manager {
	return &Manager{
		secret:     []byte(secret),
		ttl:        ttl,
		cookieName: cookieName,
		secure:     secure,
	}
}

func (m *Manager) CookieName() string { return m.cookieName }
func (m *Manager) Secure() bool       { return m.secure }

func (m *Manager) Issue(username, sub, authMethod, kind string, groups []string) (string, error) {
	now := time.Now()
	claims := Claims{
		Sub:        sub,
		Username:   username,
		Groups:     groups,
		AuthMethod: authMethod,
		Kind:       kind,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	if claims.Kind == "" {
		claims.Kind = "User"
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(m.secret)
}

func (m *Manager) Parse(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}
