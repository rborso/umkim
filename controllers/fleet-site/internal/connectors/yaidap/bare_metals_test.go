package yaidap

import (
	"strings"
	"testing"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
)

func TestComposeGuestsYAMLForSync(t *testing.T) {
	guests := `guests:
  hosts:
    vm01:
      ansible_host: 10.0.0.1
`
	bareMetals := `bare_metals:
  hosts:
    bm01:
      ansible_host: 10.0.0.2
`
	out, err := ComposeGuestsYAMLForSync(guests, bareMetals)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "vm01:") || !strings.Contains(out, "bm01:") {
		t.Fatalf("composed=%q", out)
	}
	if !strings.Contains(out, "external:") {
		t.Fatalf("expected external section: %q", out)
	}
}

func TestSplitGuestsYAMLExternal(t *testing.T) {
	raw := `guests:
  hosts:
    vm01:
      ansible_host: 10.0.0.1
external:
  hosts:
    bm01:
      ansible_host: 10.0.0.2
`
	vmOnly, bareMetals, changed, err := SplitGuestsYAMLExternal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed")
	}
	if strings.Contains(vmOnly, "external:") {
		t.Fatalf("vmOnly should not contain external: %q", vmOnly)
	}
	if !strings.Contains(bareMetals, "bm01:") {
		t.Fatalf("bareMetals=%q", bareMetals)
	}
}

func TestEnsureBareMetalsLayout(t *testing.T) {
	data := map[string]string{
		connectors.FileKey("guests.yml"): `guests:
  hosts: {}
external:
  hosts:
    bm01:
      ansible_host: 1.2.3.4
`,
	}
	put, changed := EnsureBareMetalsLayout(data)
	if !changed {
		t.Fatal("expected layout change")
	}
	if strings.Contains(put["guests.yml"], "external:") {
		t.Fatalf("guests.yml should not contain external: %q", put["guests.yml"])
	}
	if !strings.Contains(put["bare_metals.yml"], "bm01:") {
		t.Fatalf("bare_metals.yml=%q", put["bare_metals.yml"])
	}
}

func TestBareMetalsNotSynced(t *testing.T) {
	if connectors.HubCanonicalInventoryFiles["bare_metals.yml"] != true {
		t.Fatal("bare_metals.yml should be hub canonical")
	}
	data := map[string]string{
		connectors.FileKey("guests.yml"):             "guests:\n  hosts: {}\n",
		connectors.FileKey("bare_metals.yml"):        "bare_metals:\n  hosts: {}\n",
		connectors.FileKey("hypervisors.yml"):        "hypervisors:\n  hosts: {}\n",
		connectors.FileKey("group_vars/all/all.yml"): "storages: []\n",
	}
	bundle, err := connectors.BuildSyncBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bundle.Files["bare_metals.yml"]; ok {
		t.Fatal("bare_metals.yml must not be in sync bundle")
	}
}
