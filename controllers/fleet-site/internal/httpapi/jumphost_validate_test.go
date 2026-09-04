package httpapi

import "testing"

func TestValidateJumphostValue(t *testing.T) {
	if err := validateJumphostValue("jump@203.0.113.1:22"); err != nil {
		t.Fatalf("valid jumphost: %v", err)
	}
	if err := validateJumphostValue(""); err != nil {
		t.Fatalf("empty should clear: %v", err)
	}
	for _, bad := range []string{"bad", "not-valid", "@host:22", "user@"} {
		if err := validateJumphostValue(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}
