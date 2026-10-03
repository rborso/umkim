package reconcile

import (
	"errors"
	"testing"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
)

func TestClassifyACMEErrorDNS(t *testing.T) {
	if classifyACMEError(errors.New("acme: dns problem")) != acmeErrorDNS {
		t.Fatal("expected DNS class")
	}
	if tlsPhaseForClass(acmeErrorDNS) != fleetv1alpha1.TLSPhaseFailedDNS {
		t.Fatal("wrong phase")
	}
}

func TestBackoffRateLimit(t *testing.T) {
	d := backoffForAttempt(acmeErrorRateLimit, 2)
	if d < time.Hour {
		t.Fatalf("expected >= 1h, got %s", d)
	}
}
