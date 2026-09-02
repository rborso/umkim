package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Namespace                         string
	HubWireguardIP                    string
	HubLanAPI                         string
	HubWireguardPublicKey             string
	EnrollmentBaseURL                 string
	WireguardEndpointHost             string
	WireguardEndpointPort             string
	WireguardKeepalive                string
	WireguardSubnet                   string
	ReservedOverlayHosts              []string
	PoolStart                         int
	PoolEnd                           int
	AnsibleRunnerImage                string
	HubHostInventoryName              string
	SSHUser                           string
	RequeueIntervalSec                int
	EdgeDomainsCMNamespace            string
	EdgeDomainsCMName                 string
	EdgeClustersCMNamespace           string
	EdgeClustersCMName                string
	HubEdgeDomain                     string
	HubIngressAuthLDAPBaseDN          string
	HubPublicIPv4                     string
	FleetEdgeACMEEnabled              bool
	FleetEdgeACMEEmail                string
	FleetIngressAuthEnabled           bool
	FleetIngressAuthBootstrapPassword string
	FleetFCJDefaultAcmeEnabled        bool
	FleetFCJDefaultIngressAuthEnabled bool
	FleetLLMEnabled                   bool
	EdgeRoutesNamespace               string
	ManagementSiteID                  string
	ManagementClusterName             string
	DefaultRockyMajor                 string
	ConsoleNamespace                  string
	LldapWebUINodePort                int
	// RequireIdentity when true rejects mutating API calls without X-Fleet-Subject-* headers.
	RequireIdentity bool
	// HelmOCIPrefixes is a comma-separated allowlist of oci:// chart prefixes.
	HelmOCIPrefixes string
	// ChartInboxDir is the hub hostPath for uploaded chart tarballs.
	ChartInboxDir string
}

func FromEnv() Config {
	return Config{
		Namespace:                         env("FLEET_SITE_NAMESPACE", "fleet-system"),
		HubWireguardIP:                    env("FLEET_SITE_HUB_WG_IP", "10.10.0.1"),
		HubLanAPI:                         env("FLEET_SITE_HUB_LAN_API", "https://198.51.100.10:6443"),
		HubWireguardPublicKey:             env("FLEET_SITE_HUB_WG_PUBLIC_KEY", ""),
		EnrollmentBaseURL:                 env("FLEET_SITE_ENROLLMENT_BASE_URL", "http://10.10.0.1:30912/fleet/api/v1alpha1"),
		WireguardEndpointHost:             env("FLEET_SITE_WG_ENDPOINT_HOST", "wg.umkim.com"),
		WireguardEndpointPort:             env("FLEET_SITE_WG_ENDPOINT_PORT", "51820"),
		WireguardKeepalive:                env("FLEET_SITE_WG_KEEPALIVE", "25"),
		WireguardSubnet:                   env("FLEET_SITE_WG_SUBNET", "10.10.0.0/16"),
		ReservedOverlayHosts:              parseReservedHosts(env("FLEET_SITE_WG_RESERVED", "")),
		PoolStart:                         envInt("FLEET_SITE_WG_POOL_START", 10),
		PoolEnd:                           envInt("FLEET_SITE_WG_POOL_END", 99),
		AnsibleRunnerImage:                env("FLEET_SITE_ANSIBLE_RUNNER_IMAGE", "fleet/fleet-ansible-runner:0.1.0"),
		HubHostInventoryName:              env("FLEET_SITE_HUB_INVENTORY_NAME", "umkim-hub"),
		SSHUser:                           env("FLEET_SITE_SSH_USER", "linux"),
		RequeueIntervalSec:                envInt("FLEET_SITE_REQUEUE_SEC", 30),
		EdgeDomainsCMNamespace:            env("FLEET_EDGE_DOMAINS_CM_NS", "fleet-system"),
		EdgeDomainsCMName:                 env("FLEET_EDGE_DOMAINS_CM_NAME", "fleet-edge-domains"),
		EdgeClustersCMNamespace:           env("FLEET_EDGE_CLUSTERS_CM_NS", "fleet-system"),
		EdgeClustersCMName:                env("FLEET_EDGE_CLUSTERS_CM_NAME", "fleet-edge-clusters"),
		HubEdgeDomain:                     env("FLEET_HUB_EDGE_DOMAIN", "mgmt.umkim.com"),
		HubIngressAuthLDAPBaseDN:          env("FLEET_INGRESS_AUTH_LDAP_BASE_DN", "dc=umkim,dc=com"),
		HubPublicIPv4:                     env("FLEET_SITE_HUB_PUBLIC_IPV4", ""),
		FleetEdgeACMEEnabled:              envBool("FLEET_EDGE_ACME_ENABLED", true),
		FleetEdgeACMEEmail:                env("FLEET_EDGE_ACME_EMAIL", "fleet@umkim.com"),
		FleetIngressAuthEnabled:           envBool("FLEET_INGRESS_AUTH_ENABLED", false),
		FleetIngressAuthBootstrapPassword: env("FLEET_INGRESS_AUTH_BOOTSTRAP_PASSWORD", ""),
		FleetFCJDefaultAcmeEnabled:        envBool("FLEET_FCJ_DEFAULT_ACME_ENABLED", false),
		FleetFCJDefaultIngressAuthEnabled: envBool("FLEET_FCJ_DEFAULT_INGRESS_AUTH_ENABLED", false),
		FleetLLMEnabled:                   envBool("FLEET_LLM_ENABLED", false),
		EdgeRoutesNamespace:               env("FLEET_ROUTE_NAMESPACE", "fleet-system"),
		ManagementSiteID:                  env("FLEET_MANAGEMENT_SITE_ID", "umkim"),
		ManagementClusterName:             env("FLEET_MANAGEMENT_CLUSTER_NAME", "umkim-hub"),
		DefaultRockyMajor:                 env("FLEET_SITE_ROCKY_MAJOR", "10"),
		ConsoleNamespace:                  env("FLEET_CONSOLE_NAMESPACE", "fleet-console"),
		LldapWebUINodePort:                envInt("FLEET_LLAP_WEBUI_NODEPORT", 30917),
		RequireIdentity:                   envBool("FLEET_SITE_REQUIRE_IDENTITY", true),
		HelmOCIPrefixes:                   env("FLEET_HELM_OCI_PREFIXES", ""),
		ChartInboxDir:                     env("FLEET_CHART_INBOX", "/var/lib/umkim/chart-inbox"),
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return def
}

// RockyMajor returns the EL major version for bootstrap asserts (default 10).
func (c Config) RockyMajor(spec string) string {
	v := strings.TrimSpace(spec)
	if v == "" {
		v = strings.TrimSpace(c.DefaultRockyMajor)
	}
	if v == "" {
		v = "10"
	}
	major, _, _ := strings.Cut(v, ".")
	if major == "" {
		return "10"
	}
	return major
}

func (c Config) harborChartsPrefix() string {
	host := strings.TrimSpace(c.HubWireguardIP)
	if host == "" {
		host = "10.10.0.1"
	}
	return "oci://" + host + ":30443/charts"
}

// DefaultChartPrefix is the Harbor charts OCI prefix used to expand short chart names.
func (c Config) DefaultChartPrefix() string {
	for _, p := range c.ChartOCIPrefixes() {
		if p == "*" {
			continue
		}
		return p
	}
	return c.harborChartsPrefix()
}

// ChartOCIPrefixes returns allowed oci:// prefixes. "*" allows any oci:// chart.
func (c Config) ChartOCIPrefixes() []string {
	raw := strings.TrimSpace(c.HelmOCIPrefixes)
	if raw == "" {
		raw = c.harborChartsPrefix()
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, strings.TrimRight(p, "/"))
		}
	}
	if len(out) == 0 {
		return []string{c.harborChartsPrefix()}
	}
	return out
}

// HarborRegistryHost is host:port for Harbor HTTPS NodePort.
func (c Config) HarborRegistryHost() string {
	host := strings.TrimSpace(c.HubWireguardIP)
	if host == "" {
		host = "10.10.0.1"
	}
	return host + ":30443"
}

// HarborChartsProject is the hosted OCI chart project name.
func (c Config) HarborChartsProject() string {
	return "charts"
}

// HarborMirrorProject is the hosted project for copied chart images.
func (c Config) HarborMirrorProject() string {
	return "mirror"
}

func (c Config) InboxDir() string {
	d := strings.TrimSpace(c.ChartInboxDir)
	if d == "" {
		return "/var/lib/umkim/chart-inbox"
	}
	return d
}

func parseReservedHosts(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "10.10.0.1,10.10.0.2,10.10.0.3,10.10.0.11"
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
