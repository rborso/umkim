package config

import (
	"os"
	"strconv"
)

type Config struct {
	ClusterConfigMapNS      string
	ClusterConfigMapName    string
	FleetRoutesCMNS         string
	FleetRoutesCMName       string
	FleetRoutesCMKey        string
	FleetEdgeDomain         string
	SpokeIngressNS          string
	ACMEEnabled             bool
	ACMEPreflightEnabled    bool
	GatewayProbeHost        string
	RequeueSeconds          int
	CatalogAPIURL           string
	RouteSyncIntervalSec    int
	RouteNamespace          string
	HubGatewayURL           string
	HubGatewayHTTPURL       string
	RouteHealthIntervalSec  int
	HealthFailureThreshold  int
	HealthSuccessThreshold  int
	AutheliaEnabled         bool
	AutheliaForwardAuthURL  string
	AutheliaPortalHost      string
	AutheliaServiceURL      string
	GatewayDeploymentNS     string
	GatewayDeploymentName string
	ACMEQueueRetryIntervalSec int
	ACMESolverEnabled           bool
	ACMELegacyGatewayScale      bool
	ACMESolverDeploymentName    string
	ACMESolverServiceName       string
	ACMESolverRoutesCMName      string
	ACMESolverRoutesCMKey       string
	ACMEDebounceSec             int
	ACMEDNSCheckEnabled         bool
	SpokeACMEPreflightEnabled   bool
	GatewayWANIP                string
}

func FromEnv() Config {
	return Config{
		ClusterConfigMapNS:   env("FLEET_EDGE_CLUSTER_CM_NS", "fleet-system"),
		ClusterConfigMapName: env("FLEET_EDGE_CLUSTER_CM_NAME", "fleet-edge-clusters"),
		FleetRoutesCMNS:      env("FLEET_ROUTES_CM_NS", "fleet-ingress"),
		FleetRoutesCMName:    env("FLEET_ROUTES_CM_NAME", "fleet-traefik-fleet-routes"),
		FleetRoutesCMKey:     env("FLEET_ROUTES_CM_KEY", "fleet-routes.yml"),
		FleetEdgeDomain:      env("FLEET_EDGE_DOMAIN", "mgmt.umkim.com"),
		SpokeIngressNS:       env("FLEET_SPOKE_INGRESS_NS", "fleet-ingress"),
		ACMEEnabled:          envBool("FLEET_EDGE_ACME_ENABLED", false),
		ACMEPreflightEnabled: envBool("FLEET_EDGE_ACME_PREFLIGHT_ENABLED", true),
		GatewayProbeHost:     env("FLEET_GATEWAY_PROBE_HOST", ""),
		RequeueSeconds:       envInt("REQUEUE_INTERVAL_SEC", 60),
		CatalogAPIURL:        env("CATALOG_API_URL", ""),
		RouteSyncIntervalSec: envInt("ROUTE_SYNC_INTERVAL_SEC", 30),
		RouteNamespace:         env("FLEET_ROUTE_NAMESPACE", "fleet-system"),
		HubGatewayURL:          env("HUB_GATEWAY_URL", "http://127.0.0.1:80"),
		HubGatewayHTTPURL:      env("HUB_GATEWAY_HTTP_URL", ""),
		RouteHealthIntervalSec: envInt("ROUTE_HEALTH_INTERVAL_SEC", 60),
		HealthFailureThreshold: envInt("ROUTE_HEALTH_FAILURE_THRESHOLD", 3),
		HealthSuccessThreshold: envInt("ROUTE_HEALTH_SUCCESS_THRESHOLD", 2),
		AutheliaEnabled:        envBool("FLEET_INGRESS_AUTH_ENABLED", false),
		AutheliaForwardAuthURL: env("FLEET_AUTHELIA_FORWARD_AUTH_URL", "http://authelia.fleet-ingress-auth.svc.cluster.local:9091/api/authz/forward-auth"),
		AutheliaPortalHost:     env("FLEET_AUTHELIA_PORTAL_HOST", ""),
		AutheliaServiceURL:     env("FLEET_AUTHELIA_SERVICE_URL", "http://authelia.fleet-ingress-auth.svc.cluster.local:9091"),
		GatewayDeploymentNS:         env("FLEET_GATEWAY_DEPLOYMENT_NS", "fleet-ingress"),
		GatewayDeploymentName:       env("FLEET_GATEWAY_DEPLOYMENT_NAME", "gateway"),
		ACMEQueueRetryIntervalSec:   envInt("FLEET_ACME_QUEUE_RETRY_INTERVAL_SEC", 60),
		ACMESolverEnabled:           envBool("FLEET_ACME_SOLVER_ENABLED", false),
		ACMELegacyGatewayScale:      envBool("FLEET_ACME_LEGACY_GATEWAY_SCALE", false),
		ACMESolverDeploymentName:    env("FLEET_ACME_SOLVER_DEPLOYMENT", "gateway-acme"),
		ACMESolverServiceName:       env("FLEET_ACME_SOLVER_SERVICE", "gateway-acme"),
		ACMESolverRoutesCMName:      env("FLEET_ACME_SOLVER_ROUTES_CM", "fleet-traefik-acme-routes"),
		ACMESolverRoutesCMKey:       env("FLEET_ACME_SOLVER_ROUTES_CM_KEY", "fleet-routes.yml"),
		ACMEDebounceSec:             envInt("FLEET_ACME_DEBOUNCE_SEC", 30),
		ACMEDNSCheckEnabled:         envBool("FLEET_ACME_DNS_CHECK_ENABLED", true),
		SpokeACMEPreflightEnabled:   envBool("FLEET_SPOKE_ACME_PREFLIGHT_ENABLED", true),
		GatewayWANIP:                env("FLEET_GATEWAY_WAN_IP", ""),
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
		return v == "1" || v == "true" || v == "TRUE"
	}
	return def
}
