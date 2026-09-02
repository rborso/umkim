package traefik

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
	"github.com/rborso/umkim/controllers/fleet-edge/internal/clusters"
	"gopkg.in/yaml.v3"
)

const spokeInsecureTransport = "fleet-edge-spoke-insecure"

type fileConfig struct {
	HTTP httpSection `yaml:"http,omitempty"`
}

type httpSection struct {
	Routers           map[string]router               `yaml:"routers,omitempty"`
	Middlewares       map[string]middleware           `yaml:"middlewares,omitempty"`
	Services          map[string]service              `yaml:"services,omitempty"`
	ServersTransports map[string]serversTransport   `yaml:"serversTransports,omitempty"`
}

type router struct {
	Rule        string   `yaml:"rule"`
	Service     string   `yaml:"service"`
	EntryPoints []string `yaml:"entryPoints"`
	Middlewares []string `yaml:"middlewares,omitempty"`
	TLS         *tls     `yaml:"tls,omitempty"`
}

type tls struct {
	CertResolver string `yaml:"certResolver"`
}

type middleware struct {
	Headers        *headers        `yaml:"headers,omitempty"`
	ForwardAuth    *forwardAuth    `yaml:"forwardAuth,omitempty"`
	RedirectScheme *redirectScheme `yaml:"redirectScheme,omitempty"`
}

type redirectScheme struct {
	Scheme    string `yaml:"scheme"`
	Permanent bool   `yaml:"permanent"`
}

type headers struct {
	CustomRequestHeaders map[string]string `yaml:"customRequestHeaders"`
}

type forwardAuth struct {
	Address             string   `yaml:"address"`
	TrustForwardHeader  bool     `yaml:"trustForwardHeader"`
	AuthRequestHeaders  []string `yaml:"authRequestHeaders,omitempty"`
	AuthResponseHeaders []string `yaml:"authResponseHeaders,omitempty"`
}

type service struct {
	LoadBalancer *loadBalancer `yaml:"loadBalancer"`
}

type loadBalancer struct {
	Servers            []server     `yaml:"servers"`
	HealthCheck        *healthCheck `yaml:"healthCheck,omitempty"`
	PassHostHeader     *bool        `yaml:"passHostHeader,omitempty"`
	ServersTransport   string       `yaml:"serversTransport,omitempty"`
}

type serversTransport struct {
	InsecureSkipVerify bool `yaml:"insecureSkipVerify"`
}

type server struct {
	URL    string `yaml:"url"`
	Weight *int   `yaml:"weight,omitempty"`
}

type healthCheck struct {
	Path            string `yaml:"path,omitempty"`
	Interval        string `yaml:"interval,omitempty"`
	Timeout         string `yaml:"timeout,omitempty"`
	Hostname        string `yaml:"hostname,omitempty"`
	Status          int    `yaml:"status,omitempty"`
	FollowRedirects *bool  `yaml:"followRedirects,omitempty"`
}

// BuildOptions controls optional ingress auth middleware on hub-only routes.
type BuildOptions struct {
	AutheliaEnabled        bool
	AutheliaForwardAuthURL string
	AutheliaPortalHost     string
	AutheliaServiceURL     string
	// CertsReady marks hub hostnames that already have production LE certs (enables websecure routers).
	CertsReady map[string]bool
}

func certReady(opts BuildOptions, host string) bool {
	if opts.CertsReady == nil {
		return true
	}
	h := strings.ToLower(strings.TrimSpace(host))
	return opts.CertsReady[h]
}

func Build(routes []fleetv1alpha1.FleetAppRoute, reg clusters.Registry, acme bool, opts BuildOptions) ([]byte, error) {
	cfg := fileConfig{}
	cfg.HTTP.Routers = map[string]router{}
	cfg.HTTP.Middlewares = map[string]middleware{
		"fleet-edge-forward-headers": {
			Headers: &headers{CustomRequestHeaders: map[string]string{"X-Forwarded-Proto": "http"}},
		},
		"fleet-edge-forward-headers-https": {
			Headers: &headers{CustomRequestHeaders: map[string]string{"X-Forwarded-Proto": "https"}},
		},
	}
	if opts.AutheliaEnabled && opts.AutheliaForwardAuthURL != "" {
		cfg.HTTP.Middlewares["fleet-edge-authelia"] = middleware{
			ForwardAuth: &forwardAuth{
				Address:            opts.AutheliaForwardAuthURL,
				TrustForwardHeader: true,
				AuthRequestHeaders: []string{
					"Accept", "Cookie",
					"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host",
					"X-Forwarded-Uri", "X-Forwarded-Method",
				},
				AuthResponseHeaders: []string{
					"Remote-User", "Remote-Groups", "Remote-Email", "Remote-Name",
				},
			},
		}
	}
	if acme {
		cfg.HTTP.Middlewares["fleet-edge-redirect-https"] = middleware{
			RedirectScheme: &redirectScheme{Scheme: "https", Permanent: true},
		}
	}
	cfg.HTTP.Services = map[string]service{}
	needsInsecure := false
	addAutheliaPortalRoutes(&cfg, acme, opts)

	for _, route := range routes {
		if route.DeletionTimestamp != nil {
			continue
		}
		mode := effectiveRouteMode(route)
		if mode == fleetv1alpha1.RouteModeNone {
			continue
		}
		appID := sanitize(route.Spec.InstanceID)
		svcName := "fleet-app-" + appID
		servers, hc, insecure := upstreamServers(route, reg, acme)
		if len(servers) == 0 {
			continue
		}
		if insecure {
			needsInsecure = true
		}

		passHost := true
		lb := loadBalancer{Servers: servers, HealthCheck: hc, PassHostHeader: &passHost}
		if insecure {
			lb.ServersTransport = spokeInsecureTransport
		}
		cfg.HTTP.Services[svcName] = service{LoadBalancer: &lb}

		if mode == fleetv1alpha1.RouteModeHub {
			httpMids := hubRouterMiddlewares(mode, false, route.Spec.RequireAuth, opts, acme)
			cfg.HTTP.Routers["fleet-app-"+appID+"-http"] = router{
				Rule:        hostRuleHTTP(route.Spec.HubHost, acme),
				Service:     svcName,
				EntryPoints: []string{"web"},
				Middlewares: httpMids,
			}
			// TLS router must exist before cert lands in acme.json so Traefik can run TLS-ALPN issuance.
			if acme {
				cfg.HTTP.Routers["fleet-app-"+appID+"-tls"] = router{
					Rule:        hostRule(route.Spec.HubHost),
					Service:     svcName,
					EntryPoints: []string{"websecure"},
					Middlewares: hubRouterMiddlewares(mode, true, route.Spec.RequireAuth, opts, acme),
					TLS:         &tls{CertResolver: "letsencrypt"},
				}
			}
		}
	}

	if needsInsecure {
		if cfg.HTTP.ServersTransports == nil {
			cfg.HTTP.ServersTransports = map[string]serversTransport{}
		}
		cfg.HTTP.ServersTransports[spokeInsecureTransport] = serversTransport{InsecureSkipVerify: true}
	}

	if len(cfg.HTTP.Routers) == 0 && len(cfg.HTTP.Services) == 0 {
		only := struct {
			HTTP struct {
				Middlewares map[string]middleware `yaml:"middlewares"`
			} `yaml:"http"`
		}{}
		only.HTTP.Middlewares = sortedMiddlewares(cfg.HTTP.Middlewares)
		return yaml.Marshal(only)
	}

	if len(cfg.HTTP.Routers) == 0 {
		cfg.HTTP.Routers = nil
	}
	if len(cfg.HTTP.Services) == 0 {
		cfg.HTTP.Services = nil
	}
	sortHTTPSection(&cfg.HTTP)

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func failoverUpstreamHealthCheck(route fleetv1alpha1.FleetAppRoute) *healthCheck {
	followRedirects := false
	return &healthCheck{
		Path:            route.EffectiveHealthPath(),
		Interval:        "10s",
		Timeout:         "3s",
		Hostname:        route.Spec.HubHost,
		Status:          http.StatusOK,
		FollowRedirects: &followRedirects,
	}
}

func effectiveRouteMode(route fleetv1alpha1.FleetAppRoute) string {
	mode := route.Spec.EffectiveMode
	if mode == "" {
		mode = route.Spec.Mode
	}
	return mode
}

func upstreamServers(route fleetv1alpha1.FleetAppRoute, reg clusters.Registry, globalACME bool) ([]server, *healthCheck, bool) {
	primary := route.Spec.Cluster
	standby := route.Spec.StandbyCluster
	active := route.ActiveClusterName()
	highWeight, lowWeight := 10, 1
	useWeights := route.Spec.FailoverMode == fleetv1alpha1.FailoverModeStandby && standby != "" && standby != primary

	// Hub catalog hostnames always proxy to spoke Traefik hub HTTP (:30081), even when the
	// spoke cluster uses acme_cluster site TLS. effectiveMode tracks routing policy, not upstream scheme.
	upstreamMode := effectiveRouteMode(route)
	if route.Spec.Mode == fleetv1alpha1.RouteModeHub {
		upstreamMode = fleetv1alpha1.RouteModeHub
	}

	var servers []server
	needsInsecure := false
	addCluster := func(clusterName string) {
		c, ok := reg.Get(clusterName)
		if !ok {
			return
		}
		url := clusterUpstreamURL(c, globalACME, upstreamMode)
		if strings.HasPrefix(url, "https://") {
			needsInsecure = true
		}
		srv := server{URL: url}
		if useWeights {
			w := lowWeight
			if clusterName == active {
				w = highWeight
			}
			srv.Weight = &w
		}
		servers = append(servers, srv)
	}
	addCluster(primary)
	if standby != "" && standby != primary {
		addCluster(standby)
	}
	var hc *healthCheck
	if len(servers) > 1 {
		hc = failoverUpstreamHealthCheck(route)
	}
	return servers, hc, needsInsecure
}

func clusterUpstreamURL(c clusters.Cluster, globalACME bool, routeMode string) string {
	// Hub catalog hostnames are programmed on spoke Ingress :80 only; HTTPS entrypoint serves site zones.
	if routeMode == fleetv1alpha1.RouteModeHub {
		port := c.TraefikHTTPPort
		if c.SpokeACMEEnabled(globalACME) {
			port = c.TraefikHubHTTPPort
			if port == 0 {
				port = 30081
			}
		}
		if port == 0 {
			port = 30080
		}
		return fmt.Sprintf("http://%s:%d", c.UpstreamHost, port)
	}
	if c.SpokeACMEEnabled(globalACME) {
		port := c.TraefikHTTPSPort
		if port == 0 {
			port = 30443
		}
		return fmt.Sprintf("https://%s:%d", c.UpstreamHost, port)
	}
	return fmt.Sprintf("http://%s:%d", c.UpstreamHost, c.TraefikHTTPPort)
}

func hubRouterMiddlewares(mode string, https bool, requireAuth bool, opts BuildOptions, acme bool) []string {
	if acme && !https {
		return []string{"fleet-edge-redirect-https"}
	}
	headerMid := "fleet-edge-forward-headers"
	if https {
		headerMid = "fleet-edge-forward-headers-https"
	}
	if mode == fleetv1alpha1.RouteModeHub && requireAuth && opts.AutheliaEnabled && opts.AutheliaForwardAuthURL != "" {
		return []string{headerMid, "fleet-edge-authelia"}
	}
	return []string{headerMid}
}

func hostRule(host string) string {
	return fmt.Sprintf("Host(`%s`)", host)
}

// hostRuleHTTP builds the HTTP entrypoint rule. When ACME is enabled, exclude
// /.well-known/acme-challenge/ so HTTP-01 can complete before redirect-to-HTTPS.
func hostRuleHTTP(host string, acme bool) string {
	rule := hostRule(host)
	if acme {
		rule += " && !PathPrefix(`/.well-known/acme-challenge/`)"
	}
	return rule
}

func addAutheliaPortalRoutes(cfg *fileConfig, acme bool, opts BuildOptions) {
	if !opts.AutheliaEnabled || opts.AutheliaPortalHost == "" || opts.AutheliaServiceURL == "" {
		return
	}
	cfg.HTTP.Services["fleet-authelia-portal"] = service{
		LoadBalancer: &loadBalancer{
			Servers: []server{{URL: opts.AutheliaServiceURL}},
		},
	}
	portalHTTPMids := []string{"fleet-edge-forward-headers"}
	if acme {
		portalHTTPMids = []string{"fleet-edge-redirect-https"}
	}
	cfg.HTTP.Routers["fleet-authelia-portal"] = router{
		Rule:        hostRuleHTTP(opts.AutheliaPortalHost, acme),
		Service:     "fleet-authelia-portal",
		EntryPoints: []string{"web"},
		Middlewares: portalHTTPMids,
	}
	// TLS router must exist before cert lands in acme.json so Traefik can run TLS-ALPN issuance.
	if acme {
		cfg.HTTP.Routers["fleet-authelia-portal-tls"] = router{
			Rule:        hostRule(opts.AutheliaPortalHost),
			Service:     "fleet-authelia-portal",
			EntryPoints: []string{"websecure"},
			Middlewares: []string{"fleet-edge-forward-headers-https"},
			TLS:         &tls{CertResolver: "letsencrypt"},
		}
	}
}

func sanitize(id string) string {
	s := strings.ToLower(id)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "route"
	}
	return out
}

func sortHTTPSection(http *httpSection) {
	if http == nil {
		return
	}
	http.Routers = sortedRouters(http.Routers)
	http.Middlewares = sortedMiddlewares(http.Middlewares)
	http.Services = sortedServices(http.Services)
	http.ServersTransports = sortedServersTransports(http.ServersTransports)
}

func sortedStringMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedRouters(m map[string]router) map[string]router {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]router, len(m))
	for _, k := range sortedStringMapKeys(m) {
		out[k] = m[k]
	}
	return out
}

func sortedMiddlewares(m map[string]middleware) map[string]middleware {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]middleware, len(m))
	for _, k := range sortedStringMapKeys(m) {
		out[k] = m[k]
	}
	return out
}

func sortedServices(m map[string]service) map[string]service {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]service, len(m))
	for _, k := range sortedStringMapKeys(m) {
		out[k] = m[k]
	}
	return out
}

func sortedServersTransports(m map[string]serversTransport) map[string]serversTransport {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]serversTransport, len(m))
	for _, k := range sortedStringMapKeys(m) {
		out[k] = m[k]
	}
	return out
}
