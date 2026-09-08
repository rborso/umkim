package yaidap

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	fleetv1alpha1 "github.com/rborso/umkim/controllers/fleet-site/api/v1alpha1"
	"gopkg.in/yaml.v3"
)

// ParseImportJSON decodes connector import_inventory stdout.
func ParseImportJSON(raw string) (connectors.ImportResult, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return connectors.ImportResult{}, fmt.Errorf("empty import payload")
	}
	var out connectors.ImportResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return connectors.ImportResult{}, fmt.Errorf("invalid import JSON: %w", err)
	}
	return out, nil
}

// MergeImport applies pulled jumphost inventory into hub guest/catalog maps.
func MergeImport(existingCatalog Catalog, existingGuests GuestStore, pulled connectors.ImportResult, merge bool) (Catalog, GuestStore, error) {
	catalog := existingCatalog
	guests := existingGuests
	if guests == nil {
		guests = GuestStore{}
	}
	if pulled.Catalog != nil && len(pulled.Catalog) > 0 {
		raw, err := json.Marshal(pulled.Catalog)
		if err != nil {
			return catalog, guests, err
		}
		decoded, err := DecodeCatalog(string(raw))
		if err != nil {
			return catalog, guests, err
		}
		if merge {
			catalog = mergeCatalog(catalog, decoded)
		} else {
			catalog = decoded
		}
	}
	for name, fields := range pulled.Guests {
		spec, err := guestSpecFromMap(fields)
		if err != nil {
			return catalog, guests, fmt.Errorf("guest %q: %w", name, err)
		}
		if merge {
			merged := mergeGuestSpec(guests[name], spec)
			guests[name] = merged
		} else {
			guests[name] = spec
		}
	}
	if err := ValidateCatalog(&catalog); err != nil {
		return catalog, guests, err
	}
	if err := ValidateGuestStore(guests); err != nil {
		return catalog, guests, err
	}
	return catalog, guests, nil
}

func mergeCatalog(dst, src Catalog) Catalog {
	if len(src.Networks) > 0 {
		dst.Networks = src.Networks
	}
	if len(src.Storages) > 0 {
		dst.Storages = src.Storages
	}
	if len(src.Hypervisors) > 0 {
		dst.Hypervisors = src.Hypervisors
	}
	if src.Defaults != (GuestDefaults{}) {
		dst.Defaults = src.Defaults
	}
	return dst
}

func guestSpecFromMap(fields map[string]interface{}) (GuestSpec, error) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return GuestSpec{}, err
	}
	var spec GuestSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return GuestSpec{}, err
	}
	return spec, nil
}

// HandoffFromGuest builds upward provision handoff from imported guest fields.
func HandoffFromGuest(name string, spec GuestSpec, siteID string) fleetv1alpha1.ProvisionHandoffHost {
	osFamily := strings.TrimSpace(spec.OS)
	if osFamily == "" {
		osFamily = "rocky"
	}
	return fleetv1alpha1.ProvisionHandoffHost{
		Hostname:    name,
		AnsibleHost: strings.TrimSpace(spec.AnsibleHost),
		AnsibleUser: "linux",
		OSFamily:    osFamily,
		SiteID:      siteID,
	}
}

// GuestsYAMLToStore converts guests.yml into legacy guest store entries.
func GuestsYAMLToStore(raw string) (GuestStore, error) {
	var doc GuestsYAML
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("invalid guests.yml: %w", err)
	}
	out := GuestStore{}
	for name, entry := range doc.Guests.Hosts {
		out[name] = GuestSpec{
			AnsibleHost: strings.TrimSpace(entry.AnsibleHost),
			VmLocation:  strings.TrimSpace(entry.VmLocation),
		}
	}
	return out, nil
}

// ParseHostVarsYAML imports one host_vars file into a guest spec merge patch.
func ParseHostVarsYAML(raw string) (GuestSpec, error) {
	var fields map[string]interface{}
	if err := yaml.Unmarshal([]byte(raw), &fields); err != nil {
		return GuestSpec{}, fmt.Errorf("invalid host_vars: %w", err)
	}
	return guestSpecFromMap(fields)
}
