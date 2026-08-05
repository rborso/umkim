package hubwan

import "testing"

func TestInvalid(t *testing.T) {
	for _, ip := range []string{"", "127.0.0.1", "localhost", "LOCALHOST"} {
		if !Invalid(ip) {
			t.Fatalf("%q should be invalid", ip)
		}
	}
	if Invalid("203.0.113.1") {
		t.Fatal("public IP should be valid")
	}
}

func TestInventoryYAMLLines(t *testing.T) {
	lines := InventoryYAMLLines("203.0.113.5")
	if len(lines) != 4 {
		t.Fatalf("want 4 lines, got %d", len(lines))
	}
	if lines[0] != `fleet_hub_public_ipv4: "203.0.113.5"` {
		t.Fatalf("unexpected: %s", lines[0])
	}
	if len(InventoryYAMLLines("")) != 0 {
		t.Fatal("empty wan should emit no lines")
	}
}
