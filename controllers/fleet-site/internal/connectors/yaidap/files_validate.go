package yaidap

import (
	"fmt"
	"strings"

	"github.com/rborso/umkim/controllers/fleet-site/internal/connectors"
	"gopkg.in/yaml.v3"
)

// ValidateInventoryFile validates YAML content for a logical inventory path.
func ValidateInventoryFile(path, content string) error {
	path = connectors.NormalizeInventoryPath(path)
	if connectors.LegacyClusterInventoryPath(path) {
		return fmt.Errorf("%s is deprecated; use root %s.yml for membership and group_vars/%[1]s.yml for Fleet intent", path, strings.TrimSuffix(strings.TrimPrefix(path, "clusters/"), ".yml"))
	}
	if !connectors.InventoryPathAllowed(path) {
		if path == "catalog.yml" {
			return fmt.Errorf("catalog.yml is deprecated; use group_vars/all/all.yml and hypervisors.yml")
		}
		return fmt.Errorf("unsupported inventory path %q", path)
	}
	switch path {
	case "guests.yml":
		return validateGuestsYAML(content)
	case "hypervisors.yml":
		return validateHypervisorsYAML(content)
	case "":
		return connectors.ErrInvalidInventoryPath
	default:
		if connectors.IsRootInventoryGroupFile(path) {
			return validateRootInventoryGroupFile(path, content, "")
		}
		if strings.HasPrefix(path, "group_vars/") && strings.HasSuffix(path, ".yml") {
			return validateGroupVarsYAML(path, content)
		}
		if strings.HasPrefix(path, "host_vars/") && strings.HasSuffix(path, ".yml") {
			return validateHostVarsYAML(path, content, "", "")
		}
		if path == "_meta/status.json" {
			return validateStatusJSON(content)
		}
		return fmt.Errorf("unsupported inventory path %q", path)
	}
}

func validateGuestsYAML(content string) error {
	var doc GuestsYAML
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return fmt.Errorf("guests.yml: %w", err)
	}
	allowedGuestKeys := map[string]bool{"ansible_host": true, "vm_location": true}
	for name, entry := range doc.Guests.Hosts {
		if err := ValidateGuestName(name); err != nil {
			return fmt.Errorf("guests.hosts.%s: %w", name, err)
		}
		if err := validateThinGuestEntry(name, entry, allowedGuestKeys); err != nil {
			return err
		}
	}
	for name, entry := range doc.External.Hosts {
		if err := ValidateGuestName(name); err != nil {
			return fmt.Errorf("external.hosts.%s: %w", name, err)
		}
		if err := validateThinGuestEntry(name, entry, allowedGuestKeys); err != nil {
			return err
		}
	}
	return nil
}

type hypervisorsYAML struct {
	Hypervisors struct {
		Hosts map[string]struct {
			AnsibleHost string `yaml:"ansible_host"`
		} `yaml:"hosts"`
	} `yaml:"hypervisors"`
}

func validateHypervisorsYAML(content string) error {
	var doc hypervisorsYAML
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return fmt.Errorf("hypervisors.yml: %w", err)
	}
	if doc.Hypervisors.Hosts == nil {
		return fmt.Errorf("hypervisors.yml: hypervisors.hosts required")
	}
	for name, entry := range doc.Hypervisors.Hosts {
		if err := ValidateGuestName(name); err != nil {
			return fmt.Errorf("hypervisors.hosts.%s: %w", name, err)
		}
		if strings.TrimSpace(entry.AnsibleHost) == "" {
			return fmt.Errorf("hypervisors.hosts.%s: ansible_host required", name)
		}
	}
	return nil
}

func validateGroupVarsYAML(path, content string) error {
	var doc interface{}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func validateThinGuestEntry(name string, entry GuestEntry, allowed map[string]bool) error {
	raw, err := yaml.Marshal(entry)
	if err != nil {
		return err
	}
	var m map[string]interface{}
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return err
	}
	for k := range m {
		if !allowed[k] {
			return fmt.Errorf("guests.yml entry %q: field %q belongs in host_vars/%s.yml", name, k, name)
		}
	}
	if ip := strings.TrimSpace(entry.AnsibleHost); ip != "" {
		spec := GuestSpec{AnsibleHost: ip}
		if err := ValidateGuestSpec(name, spec); err != nil {
			return err
		}
	}
	return nil
}

func validateHostVarsYAML(path, content, guestsYAML, hypervisorsYAML string) error {
	name := strings.TrimSuffix(strings.TrimPrefix(path, "host_vars/"), ".yml")
	if err := ValidateGuestName(name); err != nil {
		return err
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if guestsYAML != "" || hypervisorsYAML != "" {
		inGuests := false
		if guestsYAML != "" {
			guests, err := ParseGuestsYAML(guestsYAML)
			if err != nil {
				return err
			}
			inGuests = guestInIndex(name, guests)
		}
		inHypervisors := false
		if hypervisorsYAML != "" {
			hvs, err := ParseHypervisorsYAML(hypervisorsYAML)
			if err != nil {
				return err
			}
			inHypervisors = hypervisorInIndex(name, hvs)
		}
		if (guestsYAML != "" || hypervisorsYAML != "") && !inGuests && !inHypervisors {
			return fmt.Errorf("%s: host %q not listed in guests.yml, external.hosts, or hypervisors.yml", path, name)
		}
	}
	return nil
}

func hypervisorInIndex(name string, doc hypervisorsYAML) bool {
	if doc.Hypervisors.Hosts == nil {
		return false
	}
	_, ok := doc.Hypervisors.Hosts[name]
	return ok
}

func ParseHypervisorsYAML(content string) (hypervisorsYAML, error) {
	var doc hypervisorsYAML
	if strings.TrimSpace(content) == "" {
		return doc, nil
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return hypervisorsYAML{}, fmt.Errorf("hypervisors.yml: %w", err)
	}
	return doc, nil
}

func guestInIndex(name string, doc GuestsYAML) bool {
	if doc.Guests.Hosts != nil {
		if _, ok := doc.Guests.Hosts[name]; ok {
			return true
		}
	}
	if doc.External.Hosts != nil {
		if _, ok := doc.External.Hosts[name]; ok {
			return true
		}
	}
	return false
}

// ParseGuestsYAML parses guests.yml content.
func ParseGuestsYAML(content string) (GuestsYAML, error) {
	var doc GuestsYAML
	if strings.TrimSpace(content) == "" {
		return doc, nil
	}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return GuestsYAML{}, fmt.Errorf("guests.yml: %w", err)
	}
	return doc, nil
}

func validateStatusJSON(content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	var v interface{}
	if err := jsonDecode([]byte(content), &v); err != nil {
		return fmt.Errorf("status.json: %w", err)
	}
	return nil
}

// ValidateInventoryFileWithContext validates a file with cross-file checks.
func ValidateInventoryFileWithContext(path, content, guestsYAML, hypervisorsYAML string) error {
	path = connectors.NormalizeInventoryPath(path)
	if connectors.LegacyClusterInventoryPath(path) {
		if path == "clusters.yml" {
			return validateClustersYAML(content, guestsYAML)
		}
		return validateLegacyClusterFile(path, content, guestsYAML)
	}
	if strings.HasPrefix(path, "host_vars/") {
		return validateHostVarsYAML(path, content, guestsYAML, hypervisorsYAML)
	}
	if connectors.IsRootInventoryGroupFile(path) {
		return validateRootInventoryGroupFile(path, content, guestsYAML)
	}
	if strings.HasPrefix(path, "group_vars/") && strings.HasSuffix(path, ".yml") {
		return validateGroupVarsYAML(path, content)
	}
	return ValidateInventoryFile(path, content)
}
