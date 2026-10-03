package llmkeys

import "testing"

func TestModelAllowed(t *testing.T) {
	if !ModelAllowed(nil, "any") {
		t.Fatal("empty allow list should allow all")
	}
	if !ModelAllowed([]string{"m1"}, "m1") {
		t.Fatal("exact match")
	}
	if ModelAllowed([]string{"m1"}, "m2") {
		t.Fatal("should deny")
	}
}

func TestPrefixOf(t *testing.T) {
	if got := prefixOf("fleet_llm_abc"); got == "" {
		t.Fatal("prefix")
	}
}
