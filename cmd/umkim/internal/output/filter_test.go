package output

import (
	"encoding/json"
	"testing"
)

func TestNormalizeCapabilityRequirement(t *testing.T) {
	cases := map[string]string{
		"gpu":         "gpu",
		"KVM":         "kvm",
		"ha":          "ha_topology",
		"ha-topology": "ha_topology",
		"rwx":         "storage.rwx",
		"storage.rwo": "storage.rwo",
	}
	for in, want := range cases {
		got, err := NormalizeCapabilityRequirement(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestParseRequireFlags(t *testing.T) {
	got, err := ParseRequireFlags([]string{"gpu,kvm", "ha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != "gpu" || got[1] != "kvm" || got[2] != "ha_topology" {
		t.Fatalf("got %#v", got)
	}
}

func TestFilterClustersJSON(t *testing.T) {
	raw := []byte(`{"clusters":[
		{"name":"a","available":true,"provisioning_phase":"Ready","capabilities":{"gpu":true,"kvm":false}},
		{"name":"b","available":true,"provisioning_phase":"Ready","capabilities":{"gpu":true,"kvm":true}},
		{"name":"c","available":false,"provisioning_phase":"Pending","capabilities":{"gpu":true,"kvm":true}}
	]}`)
	out, err := FilterClustersJSON(raw, []string{"gpu", "kvm"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Clusters []map[string]interface{} `json:"clusters"`
	}
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Clusters) != 1 || body.Clusters[0]["name"] != "b" {
		t.Fatalf("got %#v", body.Clusters)
	}
}
