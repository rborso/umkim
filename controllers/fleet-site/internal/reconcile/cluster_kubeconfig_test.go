package reconcile

import "testing"

func TestFleetKubeconfigSecretName(t *testing.T) {
	if got := fleetKubeconfigSecretName("lab-ha-0"); got != "fleet-kubeconfig-lab-ha-0" {
		t.Fatalf("unexpected secret name: %q", got)
	}
}
