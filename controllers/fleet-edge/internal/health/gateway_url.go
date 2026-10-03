package health

import (
	"strings"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
)

// HubGatewayProbeURL picks the gateway base URL for route health probes.
// HTTPS on websecure interferes with TLS-ALPN ACME; probe HTTP until the cert is Ready.
func HubGatewayProbeURL(httpsURL, httpURL, tlsPhase string) string {
	if tlsPhase == fleetv1alpha1.TLSPhaseReady {
		return httpsURL
	}
	if httpURL != "" {
		return httpURL
	}
	if strings.HasPrefix(httpsURL, "https://") {
		hostPort := strings.TrimPrefix(httpsURL, "https://")
		if strings.HasSuffix(hostPort, ":443") {
			return "http://" + strings.TrimSuffix(hostPort, ":443") + ":80"
		}
		return "http://" + hostPort
	}
	return httpsURL
}
