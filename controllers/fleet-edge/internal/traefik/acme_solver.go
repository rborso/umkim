package traefik

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// BuildACMESolverRoutes renders minimal TLS routers for TLS-ALPN issuance on gateway-acme.
func BuildACMESolverRoutes(hostnames []string, resolver string) ([]byte, error) {
	if len(hostnames) == 0 {
		return []byte("# fleet-edge: no ACME solver routes\n"), nil
	}
	cfg := fileConfig{}
	cfg.HTTP.Routers = map[string]router{}
	cfg.HTTP.Services = map[string]service{
		"acme-solver-dummy": {
			LoadBalancer: &loadBalancer{
				Servers: []server{{URL: "http://127.0.0.1:80"}},
			},
		},
	}
	for i, host := range hostnames {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		name := fmt.Sprintf("acme-solver-%d", i)
		cfg.HTTP.Routers[name] = router{
			Rule:        hostRule(host),
			Service:     "acme-solver-dummy",
			EntryPoints: []string{"websecure"},
			TLS:         &tls{CertResolver: resolver},
		}
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return out, nil
}
