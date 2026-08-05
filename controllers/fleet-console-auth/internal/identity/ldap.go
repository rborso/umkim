package identity

import (
	"context"
	"fmt"
	"strings"

	ldap "github.com/go-ldap/ldap/v3"
)

type LDAPProvider struct {
	URL         string
	BaseDN      string
	BindDN      string
	BindPassword string
	UserFilter  string
	GroupFilter string
}

func NewLDAPProvider(url, baseDN, bindDN, bindPassword, userFilter, groupFilter string) *LDAPProvider {
	return &LDAPProvider{
		URL:          url,
		BaseDN:       baseDN,
		BindDN:       bindDN,
		BindPassword: bindPassword,
		UserFilter:   userFilter,
		GroupFilter:  groupFilter,
	}
}

func (p *LDAPProvider) Name() string { return "ldap" }

func (p *LDAPProvider) Authenticate(ctx context.Context, username, password string) (*Principal, error) {
	if p.URL == "" || p.BaseDN == "" {
		return nil, fmt.Errorf("ldap not configured")
	}
	conn, err := ldap.DialURL(p.URL)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if p.BindDN != "" {
		if err := conn.Bind(p.BindDN, p.BindPassword); err != nil {
			return nil, fmt.Errorf("ldap service bind: %w", err)
		}
	}

	filter := fmt.Sprintf(p.UserFilter, ldap.EscapeFilter(username))
	search := ldap.NewSearchRequest(
		p.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 0, false,
		filter,
		[]string{"dn", "uid", "cn", "mail", "memberOf"},
		nil,
	)
	sr, err := conn.Search(search)
	if err != nil || len(sr.Entries) == 0 {
		return nil, fmt.Errorf("invalid credentials")
	}
	entry := sr.Entries[0]
	if err := conn.Bind(entry.DN, password); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	groups := memberOfGroups(entry.GetAttributeValues("memberOf"))
	if len(groups) == 0 {
		groups, _ = p.lookupGroups(conn, entry.DN, username)
	}

	display := entry.GetAttributeValue("cn")
	if display == "" {
		display = username
	}
	uid := entry.GetAttributeValue("uid")
	if uid == "" {
		uid = username
	}

	return &Principal{
		Username:    username,
		ExternalID:  uid,
		DisplayName: display,
		Groups:      groups,
		AuthMethod:  "ldap",
	}, nil
}

func (p *LDAPProvider) lookupGroups(conn *ldap.Conn, userDN, username string) ([]string, error) {
	filter := strings.ReplaceAll(p.GroupFilter, "{dn}", ldap.EscapeFilter(userDN))
	filter = strings.ReplaceAll(filter, "{username}", ldap.EscapeFilter(username))
	search := ldap.NewSearchRequest(
		p.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		filter,
		[]string{"cn"},
		nil,
	)
	sr, err := conn.Search(search)
	if err != nil {
		return nil, err
	}
	var groups []string
	for _, e := range sr.Entries {
		if cn := e.GetAttributeValue("cn"); cn != "" {
			groups = append(groups, cn)
		}
	}
	return groups, nil
}

func memberOfGroups(memberOf []string) []string {
	var out []string
	for _, dn := range memberOf {
		dn = strings.TrimSpace(dn)
		if dn == "" {
			continue
		}
		if i := strings.Index(dn, "cn="); i >= 0 {
			rest := dn[i+3:]
			if j := strings.Index(rest, ","); j >= 0 {
				out = append(out, rest[:j])
				continue
			}
		}
	}
	return out
}
