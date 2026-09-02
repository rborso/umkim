package health

import (
	"testing"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
)

func TestHubGatewayProbeURL(t *testing.T) {
	t.Parallel()
	https := "https://10.10.0.1:443"
	http := "http://10.10.0.1:80"
	if got := HubGatewayProbeURL(https, http, fleetv1alpha1.TLSPhaseReady); got != https {
		t.Fatalf("ready: got %q want %q", got, https)
	}
	if got := HubGatewayProbeURL(https, http, fleetv1alpha1.TLSPhaseIssuing); got != http {
		t.Fatalf("issuing: got %q want %q", got, http)
	}
	if got := HubGatewayProbeURL(https, "", fleetv1alpha1.TLSPhaseQueued); got != http {
		t.Fatalf("derive: got %q want %q", got, http)
	}
}
