package reconcile

import (
	"net"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-edge/internal/config"
)

const wgHubProbeFallback = "10.10.0.1"

func invalidProbeHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	return host == "" || host == "127.0.0.1" || host == "localhost"
}

func isPrivateProbeHost(host string) bool {
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}

// effectiveGatewayWANIP returns the hub WAN IPv4 used for public DNS validation.
func effectiveGatewayWANIP(cfg config.Config) string {
	if ip := strings.TrimSpace(cfg.GatewayWANIP); !invalidProbeHost(ip) {
		return ip
	}
	if ip := strings.TrimSpace(cfg.GatewayProbeHost); !invalidProbeHost(ip) {
		return ip
	}
	return ""
}

// effectiveGatewayProbeHost returns the address used for TLS probes from the controller pod.
func effectiveGatewayProbeHost(cfg config.Config) string {
	if ip := strings.TrimSpace(cfg.GatewayProbeHost); !invalidProbeHost(ip) {
		return ip
	}
	if ip := strings.TrimSpace(cfg.GatewayWANIP); !invalidProbeHost(ip) {
		return ip
	}
	return wgHubProbeFallback
}
