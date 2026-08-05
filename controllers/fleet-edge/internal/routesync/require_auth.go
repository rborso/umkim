package routesync

import (
	"strconv"
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
)

func userValueAsString(v userValue) string {
	switch x := v.Value.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return ""
	}
}

func parseRequireAuth(uv map[string]string, mode string) bool {
	if mode == fleetv1alpha1.RouteModeDual || mode == fleetv1alpha1.RouteModeNone {
		return false
	}
	raw := strings.TrimSpace(strings.ToLower(uv["routing.require_auth"]))
	return raw == "true" || raw == "1"
}
