package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Namespace              string
	APIBindAddress         string
	SessionSecret          string
	SessionTTL             time.Duration
	SessionCookieName      string
	SessionSecure          bool
	HtpasswdPath           string
	LDAPURL                string
	LDAPBaseDN             string
	LDAPBindDN             string
	LDAPBindPassword       string
	LDAPUserFilter         string
	LDAPGroupFilter        string
	OIDCIssuer             string
	OIDCClientID           string
	OIDCClientSecret       string
	OIDCRedirectURL        string
	FleetUpstream          string
	LLMUpstream            string
	LLMMasterKey           string
	LLMMasterKeySecretNS   string
	LLMMasterKeySecret     string
	LLMMasterKeySecretKey  string
	AuditRetention         int
	LoginRateLimitPerMin   int
	AuthBypass             bool
	BootstrapAdminUser     string
	BootstrapLDAPGroup     string
	BootstrapLDAPGroupCN string
	BootstrapInstallToken  string
	HtpasswdLine           string
}

func FromEnv() Config {
	ttl := 8 * time.Hour
	if v := os.Getenv("FLEET_AUTH_SESSION_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			ttl = d
		}
	}
	retention := 10000
	if v := os.Getenv("FLEET_AUTH_AUDIT_RETENTION"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			retention = n
		}
	}
	rate := 30
	if v := os.Getenv("FLEET_AUTH_LOGIN_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			rate = n
		}
	}
	return Config{
		Namespace:            firstEnv("FLEET_AUTH_NAMESPACE", "FLEET_CONSOLE_AUTH_NAMESPACE", "fleet-console"),
		APIBindAddress:       envOr("FLEET_AUTH_API_BIND", ":18090"),
		SessionSecret:        envOr("FLEET_AUTH_SESSION_SECRET", "CHANGE_ME"),
		SessionTTL:           ttl,
		SessionCookieName:    envOr("FLEET_AUTH_SESSION_COOKIE", "fleet_session"),
		SessionSecure:        strings.EqualFold(os.Getenv("FLEET_AUTH_SESSION_SECURE"), "true"),
		HtpasswdPath:         os.Getenv("FLEET_AUTH_HTPASSWD_PATH"),
		LDAPURL:              os.Getenv("FLEET_AUTH_LDAP_URL"),
		LDAPBaseDN:           os.Getenv("FLEET_AUTH_LDAP_BASE_DN"),
		LDAPBindDN:           os.Getenv("FLEET_AUTH_LDAP_BIND_DN"),
		LDAPBindPassword:     os.Getenv("FLEET_AUTH_LDAP_BIND_PASSWORD"),
		LDAPUserFilter:       envOr("FLEET_AUTH_LDAP_USER_FILTER", "(uid=%s)"),
		LDAPGroupFilter:      envOr("FLEET_AUTH_LDAP_GROUP_FILTER", "(member=%s)"),
		OIDCIssuer:           os.Getenv("FLEET_AUTH_OIDC_ISSUER"),
		OIDCClientID:         os.Getenv("FLEET_AUTH_OIDC_CLIENT_ID"),
		OIDCClientSecret:     os.Getenv("FLEET_AUTH_OIDC_CLIENT_SECRET"),
		OIDCRedirectURL:      os.Getenv("FLEET_AUTH_OIDC_REDIRECT_URL"),
		FleetUpstream:        envOr("FLEET_AUTH_FLEET_UPSTREAM", "http://fleet-site-controller.fleet-console.svc.cluster.local:18080"),
		LLMUpstream:          envOr("FLEET_AUTH_LLM_UPSTREAM", "http://litellm.fleet-llm.svc.cluster.local:4000"),
		LLMMasterKey:         os.Getenv("FLEET_AUTH_LLM_MASTER_KEY"),
		LLMMasterKeySecretNS: envOr("FLEET_AUTH_LLM_MASTER_KEY_SECRET_NS", "fleet-llm"),
		LLMMasterKeySecret:   envOr("FLEET_AUTH_LLM_MASTER_KEY_SECRET", "fleet-llm-master-key"),
		LLMMasterKeySecretKey: envOr("FLEET_AUTH_LLM_MASTER_KEY_SECRET_KEY", "master_key"),
		AuditRetention:       retention,
		LoginRateLimitPerMin: rate,
		AuthBypass:           strings.EqualFold(os.Getenv("FLEET_AUTH_BYPASS"), "true"),
		BootstrapAdminUser:   os.Getenv("FLEET_AUTH_BOOTSTRAP_ADMIN_USER"),
		BootstrapLDAPGroup:   envOr("FLEET_AUTH_BOOTSTRAP_LDAP_GROUP", "fleet-admins"),
		BootstrapLDAPGroupCN: envOr("FLEET_AUTH_BOOTSTRAP_LDAP_GROUP_CN", "fleet-admins"),
		BootstrapInstallToken: os.Getenv("FLEET_AUTH_BOOTSTRAP_INSTALL_TOKEN"),
		HtpasswdLine:         os.Getenv("FLEET_AUTH_HTPASSWD_LINE"),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func firstEnv(keys ...string) string {
	def := ""
	if len(keys) > 0 {
		def = keys[len(keys)-1]
		keys = keys[:len(keys)-1]
	}
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return def
}
