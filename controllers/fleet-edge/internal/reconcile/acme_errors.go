package reconcile

import (
	"strings"
	"time"

	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-edge/api/v1alpha1"
)

type acmeErrorClass int

const (
	acmeErrorTransient acmeErrorClass = iota
	acmeErrorDNS
	acmeErrorRateLimit
)

func classifyACMEError(err error) acmeErrorClass {
	if err == nil {
		return acmeErrorTransient
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "dns"),
		strings.Contains(msg, "nxdomain"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "faileddns"):
		return acmeErrorDNS
	case strings.Contains(msg, "429"),
		strings.Contains(msg, "ratelimit"),
		strings.Contains(msg, "rate limit"),
		strings.Contains(msg, "too many"):
		return acmeErrorRateLimit
	case strings.Contains(msg, "unauthorized"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "timeout"),
		strings.Contains(msg, "acme-challenge"):
		return acmeErrorTransient
	default:
		return acmeErrorTransient
	}
}

func tlsPhaseForClass(class acmeErrorClass) string {
	switch class {
	case acmeErrorDNS:
		return fleetv1alpha1.TLSPhaseFailedDNS
	case acmeErrorRateLimit:
		return fleetv1alpha1.TLSPhaseFailedRateLimit
	default:
		return fleetv1alpha1.TLSPhaseFailed
	}
}

func backoffForAttempt(class acmeErrorClass, attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	switch class {
	case acmeErrorDNS:
		return 24 * time.Hour
	case acmeErrorRateLimit:
		base := time.Hour
		for i := 1; i < attempts && i < 4; i++ {
			base *= 6
		}
		if base > 24*time.Hour {
			return 24 * time.Hour
		}
		return base
	default:
		if attempts >= 5 {
			return 30 * time.Minute
		}
		shift := attempts - 1
		if shift > 4 {
			shift = 4
		}
		return time.Duration(30*(1<<shift)) * time.Second
	}
}
