package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// ValidateHost resolves host and checks that at least one address matches expectedIP.
// When expectedIP is empty, any successful resolution passes.
func ValidateHost(ctx context.Context, host, expectedIP string) (bool, []string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return false, nil, fmt.Errorf("empty host")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupHost(lookupCtx, host)
	if err != nil || len(ips) == 0 {
		return false, ips, err
	}
	expectedIP = strings.TrimSpace(expectedIP)
	if expectedIP == "" {
		return true, ips, nil
	}
	for _, ip := range ips {
		if ip == expectedIP {
			return true, ips, nil
		}
	}
	return false, ips, nil
}
