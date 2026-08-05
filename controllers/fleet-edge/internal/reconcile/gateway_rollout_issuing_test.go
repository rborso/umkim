package reconcile

import (
	"testing"

	"github.com/rborso/umkim/controllers/fleet-edge/internal/config"
)

func TestACMECoordinatorIssuingFlag(t *testing.T) {
	c := newACMECoordinator(configWithACME(), &fakeACMEIssuer{})
	if c.Issuing() {
		t.Fatal("expected not issuing initially")
	}
	c.issuing.Store(true)
	if !c.Issuing() {
		t.Fatal("expected issuing after set")
	}
	c.issuing.Store(false)
	if c.Issuing() {
		t.Fatal("expected not issuing after clear")
	}
	if (*acmeCoordinator)(nil).Issuing() {
		t.Fatal("nil coordinator should not be issuing")
	}
}

func configWithACME() config.Config {
	return config.Config{ACMEEnabled: true}
}
