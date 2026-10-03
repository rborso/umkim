package cmd

import (
	"fmt"
	"strings"
	"time"
)

const (
	siteConflictRetries = 60
	siteConflictBackoff = 5 * time.Second
)

type apiCall func() ([]byte, int, error)

// retryOnHTTP409 retries fleet-site provision/inventory calls while YAIDAP holds
// a site-wide lock (HTTP 409).
func retryOnHTTP409(call apiCall) ([]byte, int, error) {
	return retryOnHTTP409WithBackoff(call, siteConflictRetries, siteConflictBackoff)
}

func retryOnHTTP409WithBackoff(call apiCall, attempts int, backoff time.Duration) ([]byte, int, error) {
	var lastRaw []byte
	var lastCode int
	for attempt := 0; attempt < attempts; attempt++ {
		raw, code, err := call()
		if err != nil {
			return raw, code, err
		}
		if code != 409 {
			return raw, code, nil
		}
		lastRaw, lastCode = raw, code
		time.Sleep(backoff)
	}
	return lastRaw, lastCode, fmt.Errorf("HTTP %d: %s", lastCode, strings.TrimSpace(string(lastRaw)))
}
