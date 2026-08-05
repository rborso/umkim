package identity

import "context"

type Principal struct {
	Username   string
	ExternalID string
	DisplayName string
	Groups     []string
	AuthMethod string
}

type IdentityProvider interface {
	Name() string
	Authenticate(ctx context.Context, username, password string) (*Principal, error)
}

type OIDCProvider interface {
	Name() string
	Enabled() bool
	AuthorizeURL(state string) string
	ExchangeCode(ctx context.Context, code string) (*Principal, error)
}
