package overlay

import (
	"testing"
)

func TestParseReservedHostsDefault(t *testing.T) {
	got := ParseReservedHosts("")
	want := []string{"10.10.0.1", "10.10.0.2", "10.10.0.3", "10.10.0.11"}
	if len(got) != len(want) {
		t.Fatalf("len %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("index %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestParseReservedHostsCustom(t *testing.T) {
	got := ParseReservedHosts(" 10.10.0.5 , 10.10.0.6 ")
	if len(got) != 2 || got[0] != "10.10.0.5" || got[1] != "10.10.0.6" {
		t.Fatalf("unexpected %v", got)
	}
}

func TestAssertAvailable(t *testing.T) {
	used := map[string]bool{"10.10.0.11": true}
	if err := AssertAvailable(used, "10.10.0.11/32"); err == nil {
		t.Fatal("expected collision for reserved IP")
	}
	if err := AssertAvailable(used, "10.10.0.20/32"); err != nil {
		t.Fatalf("expected free IP: %v", err)
	}
	if err := AssertAvailable(used, "bad"); err == nil {
		t.Fatal("expected error for invalid cidr")
	}
}

func TestMarkUsed(t *testing.T) {
	used := map[string]bool{}
	MarkUsed(used, "10.10.0.14/32")
	if !used["10.10.0.14"] {
		t.Fatal("expected host marked used")
	}
}
