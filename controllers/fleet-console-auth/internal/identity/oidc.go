package identity

import (
	"context"
	"fmt"
)

// OIDCStub implements OIDCProvider for future Keycloak integration.
type OIDCStub struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

func NewOIDCStub(issuer, clientID, clientSecret, redirectURL string) *OIDCStub {
	return &OIDCStub{
		Issuer:       issuer,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
	}
}

func (p *OIDCStub) Name() string { return "oidc" }

func (p *OIDCStub) Enabled() bool {
	return p.Issuer != "" && p.ClientID != ""
}

func (p *OIDCStub) AuthorizeURL(state string) string {
	if !p.Enabled() {
		return ""
	}
	return fmt.Sprintf("%s/protocol/openid-connect/auth?client_id=%s&response_type=code&scope=openid%%20profile%%20email&redirect_uri=%s&state=%s",
		trimSlash(p.Issuer), p.ClientID, p.RedirectURL, state)
}

func (p *OIDCStub) ExchangeCode(_ context.Context, _ string) (*Principal, error) {
	return nil, fmt.Errorf("oidc not implemented yet; configure Keycloak in a future release")
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
