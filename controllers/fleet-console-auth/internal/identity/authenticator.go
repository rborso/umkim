package identity

import (
	"context"
	"fmt"
)

type Authenticator struct {
	providers []IdentityProvider
}

func NewAuthenticator(providers ...IdentityProvider) *Authenticator {
	return &Authenticator{providers: providers}
}

func (a *Authenticator) Authenticate(ctx context.Context, username, password string) (*Principal, error) {
	var lastErr error
	for _, p := range a.providers {
		if p == nil {
			continue
		}
		principal, err := p.Authenticate(ctx, username, password)
		if err == nil {
			return principal, nil
		}
		lastErr = err
		if err.Error() != "invalid credentials" && err.Error() != "ldap not configured" && err.Error() != "htpasswd not configured" {
			return nil, err
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return nil, fmt.Errorf("no identity providers configured")
}
