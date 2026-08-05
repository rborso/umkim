package config

import (
	"os"
	"strconv"
)

type Config struct {
	RouteNamespace          string
	DegradedCMNamespace     string
	DegradedCMName          string
	PlacementManagerURL     string
	FailoverCooldownSeconds int
}

func FromEnv() Config {
	return Config{
		RouteNamespace:          env("FLEET_ROUTE_NAMESPACE", "fleet-system"),
		DegradedCMNamespace:     env("FLEET_FAILOVER_CM_NS", "fleet-system"),
		DegradedCMName:          env("FLEET_FAILOVER_CM_NAME", "fleet-failover-degraded-clusters"),
		PlacementManagerURL:     env("PLACEMENT_MANAGER_URL", "http://placement-manager.dcm.svc.cluster.local:8080/api/v1alpha1"),
		FailoverCooldownSeconds: envInt("FAILOVER_COOLDOWN_SECONDS", 300),
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
