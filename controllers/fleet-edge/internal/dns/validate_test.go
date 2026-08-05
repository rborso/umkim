package dns

import (
	"context"
	"testing"
)

func TestValidateHostEmptyExpected(t *testing.T) {
	ok, _, err := ValidateHost(context.Background(), "localhost", "")
	if err != nil || !ok {
		t.Fatalf("localhost should resolve: ok=%v err=%v", ok, err)
	}
}

func TestValidateHostMismatch(t *testing.T) {
	ok, ips, err := ValidateHost(context.Background(), "localhost", "203.0.113.1")
	if err != nil {
		t.Fatalf("lookup err: %v", err)
	}
	if ok {
		t.Fatalf("expected mismatch for localhost vs 203.0.113.1, got %v", ips)
	}
}
