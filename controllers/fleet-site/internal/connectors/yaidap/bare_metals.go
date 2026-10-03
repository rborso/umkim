package yaidap

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"gopkg.in/yaml.v3"
)

// BareMetalsYAML is the bare_metals.yml top-level structure (hub canonical).
type BareMetalsYAML struct {
	BareMetals struct {
		Hosts map[string]GuestEntry `json:"hosts" yaml:"hosts"`
	} `json:"bare_metals" yaml:"bare_metals"`
}

const emptyBareMetalsYAML = `bare_metals:
  hosts: {}
`

// ParseBareMetalsYAML parses bare_metals.yml content.
func ParseBareMetalsYAML(content string) (BareMetalsYAML, error) {
	var doc BareMetalsYAML
	if strings.TrimSpace(content) == "" {
		return doc, nil
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return BareMetalsYAML{}, fmt.Errorf("bare_metals.yml: %w", err)
	}
	return doc, nil
}

func bareMetalInIndex(name string, doc BareMetalsYAML) bool {
	if doc.BareMetals.Hosts == nil {
		return false
	}
	_, ok := doc.BareMetals.Hosts[name]
	return ok
}

// ComposeGuestsYAMLForSync builds YAIDAP-shaped guests.yml for jumphost sync.
func ComposeGuestsYAMLForSync(guestsRaw, bareMetalsRaw string) (string, error) {
	guests, err := ParseGuestsYAML(guestsRaw)
	if err != nil {
		return "", err
	}
	bareMetals, err := ParseBareMetalsYAML(bareMetalsRaw)
	if err != nil {
		return "", err
	}
	composed := GuestsYAML{}
	composed.Guests.Hosts = guests.Guests.Hosts
	composed.External.Hosts = map[string]GuestEntry{}
	for name, entry := range bareMetals.BareMetals.Hosts {
		composed.External.Hosts[name] = entry
	}
	// Read-compat: legacy external.hosts still in hub guests.yml.
	for name, entry := range guests.External.Hosts {
		if _, exists := composed.External.Hosts[name]; !exists {
			composed.External.Hosts[name] = entry
		}
	}
	return marshalGuestsYAML(composed)
}

// SplitGuestsYAMLExternal moves external.hosts out of guests.yml into bare_metals.yml.
func SplitGuestsYAMLExternal(guestsRaw string) (vmOnly string, bareMetals string, changed bool, err error) {
	doc, err := ParseGuestsYAML(guestsRaw)
	if err != nil {
		return "", "", false, err
	}
	vmOnly, err = marshalVMOnlyGuestsYAML(doc)
	if err != nil {
		return "", "", false, err
	}
	if len(doc.External.Hosts) > 0 {
		bmDoc := BareMetalsYAML{}
		bmDoc.BareMetals.Hosts = doc.External.Hosts
		bareMetals, err = marshalBareMetalsYAML(bmDoc)
		if err != nil {
			return "", "", false, err
		}
		return vmOnly, bareMetals, true, nil
	}
	changed = strings.Contains(guestsRaw, "external:")
	return vmOnly, emptyBareMetalsYAML, changed, nil
}

// NormalizeImportPut splits jumphost guests.yml external.hosts into bare_metals.yml on hub apply.
func NormalizeImportPut(put map[string]string) (map[string]string, error) {
	raw, ok := put["guests.yml"]
	if !ok || strings.TrimSpace(raw) == "" {
		return put, nil
	}
	vmOnly, bareMetals, changed, err := SplitGuestsYAMLExternal(raw)
	if err != nil {
		return nil, err
	}
	if !changed {
		return put, nil
	}
	out := map[string]string{}
	for p, content := range put {
		out[p] = content
	}
	out["guests.yml"] = vmOnly
	if existing, ok := put["bare_metals.yml"]; ok && strings.TrimSpace(existing) != "" {
		existingDoc, err := ParseBareMetalsYAML(existing)
		if err != nil {
			return nil, err
		}
		imported, err := ParseBareMetalsYAML(bareMetals)
		if err != nil {
			return nil, err
		}
		if existingDoc.BareMetals.Hosts == nil {
			existingDoc.BareMetals.Hosts = map[string]GuestEntry{}
		}
		for name, entry := range imported.BareMetals.Hosts {
			existingDoc.BareMetals.Hosts[name] = entry
		}
		merged, err := marshalBareMetalsYAML(existingDoc)
		if err != nil {
			return nil, err
		}
		out["bare_metals.yml"] = merged
	} else {
		out["bare_metals.yml"] = bareMetals
	}
	return out, nil
}

// EnsureBareMetalsLayout splits legacy external.hosts from guests.yml and ensures bare_metals.yml exists.
func EnsureBareMetalsLayout(data map[string]string) (map[string]string, bool) {
	if data == nil {
		return nil, false
	}
	data = connectors.NormalizeMetaKeys(data)
	put := map[string]string{}
	changed := false

	guestsRaw, hasGuests := connectors.GetInventoryFile(data, "guests.yml")
	if hasGuests {
		vmOnly, bareMetals, split, err := SplitGuestsYAMLExternal(guestsRaw)
		if err == nil && split {
			put["guests.yml"] = vmOnly
			changed = true
			if _, hasBM := connectors.GetInventoryFile(data, "bare_metals.yml"); hasBM {
				existing, _ := connectors.GetInventoryFile(data, "bare_metals.yml")
				existingDoc, _ := ParseBareMetalsYAML(existing)
				imported, _ := ParseBareMetalsYAML(bareMetals)
				if existingDoc.BareMetals.Hosts == nil {
					existingDoc.BareMetals.Hosts = map[string]GuestEntry{}
				}
				for name, entry := range imported.BareMetals.Hosts {
					existingDoc.BareMetals.Hosts[name] = entry
				}
				if merged, err := marshalBareMetalsYAML(existingDoc); err == nil {
					put["bare_metals.yml"] = merged
				}
			} else {
				put["bare_metals.yml"] = bareMetals
			}
		}
	}
	if _, hasBM := connectors.GetInventoryFile(data, "bare_metals.yml"); !hasBM {
		if _, pending := put["bare_metals.yml"]; !pending {
			put["bare_metals.yml"] = emptyBareMetalsYAML
			changed = true
		}
	}
	if !changed {
		return nil, false
	}
	return put, true
}

type vmOnlyGuestsYAML struct {
	Guests struct {
		Hosts map[string]GuestEntry `yaml:"hosts"`
	} `yaml:"guests"`
}

func marshalVMOnlyGuestsYAML(doc GuestsYAML) (string, error) {
	out := vmOnlyGuestsYAML{}
	out.Guests.Hosts = doc.Guests.Hosts
	if out.Guests.Hosts == nil {
		out.Guests.Hosts = map[string]GuestEntry{}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(out); err != nil {
		return "", err
	}
	_ = enc.Close()
	return strings.TrimRight(buf.String(), "\n") + "\n", nil
}

func marshalBareMetalsYAML(doc BareMetalsYAML) (string, error) {
	if doc.BareMetals.Hosts == nil {
		doc.BareMetals.Hosts = map[string]GuestEntry{}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	_ = enc.Close()
	return strings.TrimRight(buf.String(), "\n") + "\n", nil
}
