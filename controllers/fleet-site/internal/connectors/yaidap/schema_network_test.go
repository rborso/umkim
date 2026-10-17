package yaidap

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// The site catalog spells a network's name `network_name`. yaml.v3 does not
// honour `json` tags, so a struct tag of `name` left the field empty, every
// lookup missed, and physicalNetworkFor silently returned the lab fallback —
// which made a VM on any other network unresolvable at create time.
func TestCatalogNetworkLookup(t *testing.T) {
	const allYML = `
networks:
  - network_name: mgmt_nw
    cidr: 192.168.1.0/24
    physical_network: mgmt_access_l1
  - network_name: lab_nw
    cidr: 172.16.10.0/24
    physical_network: default_l2
`
	var cat Catalog
	if err := yaml.Unmarshal([]byte(allYML), &cat); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cat.Networks) != 2 {
		t.Fatalf("parsed %d networks, want 2", len(cat.Networks))
	}
	if got := cat.Networks[0].Name; got != "mgmt_nw" {
		t.Fatalf("first network name = %q, want mgmt_nw", got)
	}

	// The whole point: a named network must resolve to its physical network.
	if got := physicalNetworkFor(cat, "mgmt_nw"); got != "mgmt_access_l1" {
		t.Errorf("physicalNetworkFor(mgmt_nw) = %q, want mgmt_access_l1", got)
	}
	if got := physicalNetworkFor(cat, "lab_nw"); got != "default_l2" {
		t.Errorf("physicalNetworkFor(lab_nw) = %q, want default_l2", got)
	}
	// An unknown network keeps the documented fallback.
	if got := physicalNetworkFor(cat, "nope_nw"); got != "lab_access_l1" {
		t.Errorf("physicalNetworkFor(unknown) = %q, want the lab fallback", got)
	}
}
