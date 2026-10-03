package traefik

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	CertResolverProduction = "letsencrypt"
	CertResolverPreflight    = "letsencrypt-preflight"
)

// SetCertResolverForHostnames rewrites TLS router certResolver for routers whose
// Host() rule matches one of hostnames (case-insensitive).
func SetCertResolverForHostnames(routesYAML []byte, hostnames []string, resolver string) ([]byte, error) {
	out, _, err := ensureACMETLSRouters(routesYAML, hostnames, resolver, false, nil)
	return out, err
}

// EnsureACMETLSRouters adds websecure TLS routers for hostnames that lack one
// (deferred HTTPS) and sets certResolver so TLS-ALPN issuance can run on the main gateway.
// hostMiddlewares optionally sets middlewares on newly created pending TLS routers (hostname → middleware names).
func EnsureACMETLSRouters(routesYAML []byte, hostnames []string, resolver string) ([]byte, error) {
	return EnsureACMETLSRoutersWithMiddlewares(routesYAML, hostnames, resolver, nil)
}

func EnsureACMETLSRoutersWithMiddlewares(routesYAML []byte, hostnames []string, resolver string, hostMiddlewares map[string][]string) ([]byte, error) {
	out, _, err := ensureACMETLSRouters(routesYAML, hostnames, resolver, true, hostMiddlewares)
	return out, err
}

func ensureACMETLSRouters(routesYAML []byte, hostnames []string, resolver string, addMissing bool, hostMiddlewares map[string][]string) ([]byte, bool, error) {
	if len(hostnames) == 0 {
		return routesYAML, false, nil
	}
	want := map[string]struct{}{}
	for _, h := range hostnames {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			want[h] = struct{}{}
		}
	}
	var cfg fileConfig
	if err := yaml.Unmarshal(routesYAML, &cfg); err != nil {
		return nil, false, err
	}
	if cfg.HTTP.Routers == nil {
		cfg.HTTP.Routers = map[string]router{}
	}
	if cfg.HTTP.Services == nil {
		cfg.HTTP.Services = map[string]service{}
	}
	changed := false
	hasTLS := map[string]bool{}
	httpSvc := map[string]string{}
	for name, rt := range cfg.HTTP.Routers {
		host, ok := hostnameFromRule(rt.Rule)
		if !ok {
			continue
		}
		h := strings.ToLower(host)
		if rt.TLS != nil {
			hasTLS[h] = true
			if _, match := want[h]; !match {
				continue
			}
			if rt.TLS.CertResolver == resolver {
				continue
			}
			rt.TLS.CertResolver = resolver
			cfg.HTTP.Routers[name] = rt
			changed = true
			continue
		}
		for _, ep := range rt.EntryPoints {
			if ep == "web" {
				httpSvc[h] = rt.Service
				break
			}
		}
	}
	for _, host := range hostnames {
		h := strings.ToLower(strings.TrimSpace(host))
		if h == "" || hasTLS[h] {
			continue
		}
		if !addMissing {
			continue
		}
		svc := httpSvc[h]
		if svc == "" {
			if _, ok := cfg.HTTP.Services["acme-solver-dummy"]; !ok {
				cfg.HTTP.Services["acme-solver-dummy"] = service{
					LoadBalancer: &loadBalancer{
						Servers: []server{{URL: "http://127.0.0.1:80"}},
					},
				}
			}
			svc = "acme-solver-dummy"
		}
		name := "fleet-acme-pending-" + sanitize(h) + "-tls"
		rt := router{
			Rule:        hostRule(host),
			Service:     svc,
			EntryPoints: []string{"websecure"},
			TLS:         &tls{CertResolver: resolver},
		}
		if hostMiddlewares != nil {
			if mids, ok := hostMiddlewares[h]; ok && len(mids) > 0 {
				rt.Middlewares = append([]string(nil), mids...)
			} else if mids, ok := hostMiddlewares[host]; ok && len(mids) > 0 {
				rt.Middlewares = append([]string(nil), mids...)
			}
		}
		cfg.HTTP.Routers[name] = rt
		changed = true
	}
	if !changed {
		return routesYAML, false, nil
	}
	sortHTTPSection(&cfg.HTTP)
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func hostnameFromRule(rule string) (string, bool) {
	for _, match := range hostRulePattern.FindAllStringSubmatch(rule, -1) {
		if len(match) >= 2 {
			return strings.TrimSpace(match[1]), true
		}
	}
	return "", false
}

// HostnamesForResolver returns hostnames on TLS routers using resolver.
func HostnamesForResolver(routesYAML []byte, resolver string) ([]string, error) {
	var cfg fileConfig
	if err := yaml.Unmarshal(routesYAML, &cfg); err != nil {
		return nil, err
	}
	var hosts []string
	seen := map[string]struct{}{}
	for _, rt := range cfg.HTTP.Routers {
		if rt.TLS == nil || rt.TLS.CertResolver != resolver {
			continue
		}
		host, ok := hostnameFromRule(rt.Rule)
		if !ok {
			continue
		}
		h := strings.ToLower(host)
		if _, dup := seen[h]; dup {
			continue
		}
		seen[h] = struct{}{}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

func ValidatePreflightResolvers(routesYAML []byte, hostnames []string) error {
	hosts, err := HostnamesForResolver(routesYAML, CertResolverPreflight)
	if err != nil {
		return err
	}
	got := map[string]struct{}{}
	for _, h := range hosts {
		got[strings.ToLower(h)] = struct{}{}
	}
	for _, want := range hostnames {
		want = strings.ToLower(strings.TrimSpace(want))
		if _, ok := got[want]; !ok {
			return fmt.Errorf("preflight routes missing hostname %q", want)
		}
	}
	return nil
}
