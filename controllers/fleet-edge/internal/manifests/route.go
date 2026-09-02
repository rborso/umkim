package manifests

import (
	"fmt"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
)

	const (
	traefikMiddlewareAnnotation      = "traefik.ingress.kubernetes.io/router.middlewares"
	traefikTLSAnnotation             = "traefik.ingress.kubernetes.io/router.tls"
	traefikTLSCertResolverAnnotation = "traefik.ingress.kubernetes.io/router.tls.certresolver"
	traefikEntrypointsAnnotation     = "traefik.ingress.kubernetes.io/router.entrypoints"
	// Site routes with require_auth use spoke Traefik file-provider middleware from fleet-ingress-auth chart.
	siteAuthMiddleware = "fleet-ingress-authelia@file"
)

// SpokeIngressOptions controls spoke Ingress rendering for catalog routes.
type SpokeIngressOptions struct {
	AuthEnabled      bool
	SpokeACME        bool
	CertResolver     string
	RedirectHTTPS    bool
	UseHubEntrypoint bool
}

func RenderManifestWork(route fleetv1alpha1.FleetAppRoute, hosts []string, clusterName string, svc fleetv1alpha1.ServiceRef, opts SpokeIngressOptions) (string, error) {
	if len(hosts) == 0 {
		return "", fmt.Errorf("no ingress hosts")
	}
	svcNS := svc.Namespace
	if svcNS == "" {
		svcNS = "default"
	}
	if svc.Name == "" {
		return "", fmt.Errorf("service name required for cluster %s", clusterName)
	}
	mode := route.Spec.EffectiveMode
	if mode == "" {
		mode = route.Spec.Mode
	}
	useAuth := opts.AuthEnabled && mode == fleetv1alpha1.RouteModeSite && route.Spec.RequireAuth

	var manifestDocs strings.Builder
	if opts.SpokeACME {
		// HTTPS ingress only on web: a separate HTTP ingress with PathPrefix / steals
		// /.well-known/acme-challenge/* from Traefik's HTTP-01 handler (404).
		// HTTP→HTTPS redirect is configured on the spoke Traefik web entrypoint (phase6).
		httpsRules := ingressRules(hosts, svc.Name, svc.Port)
		httpsMids := []string{}
		if useAuth {
			httpsMids = append(httpsMids, siteAuthMiddleware)
		}
		httpsAnnotations := ingressAnnotations(ingressAnnotationOpts{
			entrypoint:   "websecure",
			tls:          true,
			certResolver: spokeCertResolver(opts),
			middlewares:  httpsMids,
		})
		tlsBlock := ingressTLSBlock(hosts)
		manifestDocs.WriteString(renderIngressManifest(
			"fleet-route-"+sanitize(route.Spec.InstanceID)+"-tls",
			svcNS, httpsAnnotations, tlsBlock, httpsRules,
		))
		// Hub Traefik proxies catalog hub hostnames to spoke :30081 (web-hub).
		// ACME TLS ingress alone leaves that entrypoint empty → hub path HTTP 404.
		if opts.UseHubEntrypoint {
			hubHosts := HubHostsForRoute(route)
			if len(hubHosts) > 0 {
				hubRules := ingressRules(hubHosts, svc.Name, svc.Port)
				hubAnnotations := ingressAnnotations(ingressAnnotationOpts{entrypoint: "web-hub"})
				manifestDocs.WriteString(renderIngressManifest(
					"fleet-route-"+sanitize(route.Spec.InstanceID)+"-hub",
					svcNS, hubAnnotations, "", hubRules,
				))
			}
		}
	} else {
		rules := ingressRules(hosts, svc.Name, svc.Port)
		annOpts := ingressAnnotationOpts{}
		if useAuth {
			annOpts.middlewares = []string{siteAuthMiddleware}
		}
		if opts.UseHubEntrypoint {
			annOpts.entrypoint = "web-hub"
		}
		annotations := ingressAnnotations(annOpts)
		manifestDocs.WriteString(renderIngressManifest(
			"fleet-route-"+sanitize(route.Spec.InstanceID),
			svcNS, annotations, "", rules,
		))
	}

	return fmt.Sprintf(`apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: %s
  namespace: %s
  labels:
    edge.umkim.com/route: "true"
    edge.umkim.com/instance-id: %q
spec:
  workload:
    manifests:
%s`, workName(route.Spec.InstanceID), clusterName, route.Spec.InstanceID, manifestDocs.String()), nil
}

type ingressAnnotationOpts struct {
	entrypoint   string
	tls          bool
	certResolver string
	middlewares  []string
}

func ingressAnnotations(opts ingressAnnotationOpts) string {
	var lines []string
	if len(opts.middlewares) > 0 {
		lines = append(lines, fmt.Sprintf(`            %s: %q`, traefikMiddlewareAnnotation, strings.Join(opts.middlewares, ",")))
	}
	if opts.entrypoint != "" {
		lines = append(lines, fmt.Sprintf(`            %s: %q`, traefikEntrypointsAnnotation, opts.entrypoint))
	}
	if opts.tls {
		lines = append(lines, fmt.Sprintf(`            %s: "true"`, traefikTLSAnnotation))
	}
	if opts.certResolver != "" {
		lines = append(lines, fmt.Sprintf(`            %s: %q`, traefikTLSCertResolverAnnotation, opts.certResolver))
	}
	if len(lines) == 0 {
		return ""
	}
	return "          annotations:\n" + strings.Join(lines, "\n") + "\n"
}

func ingressRules(hosts []string, svcName string, port int32) string {
	var rules strings.Builder
	for _, h := range hosts {
		rules.WriteString(fmt.Sprintf(`            - host: %q
              http:
                paths:
                  - path: /
                    pathType: Prefix
                    backend:
                      service:
                        name: %s
                        port:
                          number: %d
`, h, svcName, port))
	}
	return rules.String()
}

func ingressTLSBlock(hosts []string) string {
	var tlsHosts strings.Builder
	for _, h := range hosts {
		tlsHosts.WriteString(fmt.Sprintf("              - %q\n", h))
	}
	return fmt.Sprintf(`          tls:
            - hosts:
%s`, tlsHosts.String())
}

func renderIngressManifest(name, svcNS, annotations, tlsBlock, rules string) string {
	return fmt.Sprintf(`      - apiVersion: networking.k8s.io/v1
        kind: Ingress
        metadata:
          name: %s
          namespace: %s
%s        spec:
          ingressClassName: traefik
%s          rules:
%s`, name, svcNS, annotations, tlsBlock, rules)
}

func workName(instanceID string) string {
	return "fleet-route-" + sanitize(instanceID)
}

func sanitize(id string) string {
	out := make([]byte, 0, len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			out = append(out, c)
		} else {
			out = append(out, '-')
		}
	}
	s := string(out)
	if s == "" {
		return "route"
	}
	return s
}

func WorkNameForInstance(instanceID string) string {
	return workName(instanceID)
}

func HostsForRoute(route fleetv1alpha1.FleetAppRoute) []string {
	mode := route.Spec.EffectiveMode
	if mode == "" {
		mode = route.Spec.Mode
	}
	var hosts []string
	switch mode {
	case fleetv1alpha1.RouteModeHub:
		if route.Spec.HubHost != "" {
			hosts = append(hosts, route.Spec.HubHost)
		}
	case fleetv1alpha1.RouteModeSite:
		if route.Spec.SiteHost != "" {
			hosts = append(hosts, route.Spec.SiteHost)
		}
	}
	return hosts
}

// HubHostsForRoute returns hub catalog hostnames that must bind on spoke web-hub (:30081).
func HubHostsForRoute(route fleetv1alpha1.FleetAppRoute) []string {
	mode := route.Spec.EffectiveMode
	if mode == "" {
		mode = route.Spec.Mode
	}
	switch mode {
	case fleetv1alpha1.RouteModeHub:
		if route.Spec.HubHost != "" {
			return []string{route.Spec.HubHost}
		}
	}
	return nil
}

// SpokeHostsForRoute returns hostnames published on the spoke via ManifestWork Ingress.
// Hub mode proxies through spoke Traefik, so the hub hostname must exist on the spoke.
func SpokeHostsForRoute(route fleetv1alpha1.FleetAppRoute) []string {
	mode := route.Spec.EffectiveMode
	if mode == "" {
		mode = route.Spec.Mode
	}
	var hosts []string
	switch mode {
	case fleetv1alpha1.RouteModeHub:
		if route.Spec.HubHost != "" {
			hosts = append(hosts, route.Spec.HubHost)
		}
	case fleetv1alpha1.RouteModeSite:
		if route.Spec.SiteHost != "" {
			hosts = append(hosts, route.Spec.SiteHost)
		}
	}
	return hosts
}

func ValidateRouteHosts(route fleetv1alpha1.FleetAppRoute) error {
	if len(SpokeHostsForRoute(route)) == 0 {
		return nil
	}
	for _, t := range route.PlacementClusters() {
		if t.Service.Name == "" {
			return fmt.Errorf("service name required for spoke ingress on cluster %s", t.Cluster)
		}
	}
	if route.Spec.Service.Name == "" && len(route.PlacementClusters()) == 0 {
		return fmt.Errorf("service name required for spoke ingress")
	}
	return nil
}

func spokeCertResolver(opts SpokeIngressOptions) string {
	if opts.CertResolver != "" {
		return opts.CertResolver
	}
	return "letsencrypt"
}
